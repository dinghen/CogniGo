package file

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"

	"github.com/dinghen/CogniGo/common/rag"
	"github.com/dinghen/CogniGo/config"
	knowledgeDAO "github.com/dinghen/CogniGo/dao/knowledge"
	"github.com/dinghen/CogniGo/model"
	"github.com/dinghen/CogniGo/utils"
)

// 上传rag相关文件（这里只允许文本文件）
// 其实可以直接将其向量化进行保存，但这边依旧存储到服务器上以便后续可以在服务器上查看历史RAG文件
func UploadRagFile(username string, file *multipart.FileHeader) (string, error) {
	// 校验文件类型和文件名
	if err := utils.ValidateFile(file); err != nil {
		log.Printf("File validation failed: %v", err)
		return "", err
	}

	// 创建用户目录
	userDir := filepath.Join(config.GetConfig().RuntimeConfig.UploadDir, username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		log.Printf("Failed to create user directory %s: %v", userDir, err)
		return "", err
	}

	// Keep the current file until the replacement has been indexed successfully.
	// This prevents a failed embedding request from destroying the user's
	// previously working knowledge base.
	// Generate UUID as the stored filename. The original client filename is
	// used only for its validated extension, so it cannot escape userDir.
	uuid := utils.GenerateUUID()
	ext := filepath.Ext(file.Filename)
	filename := uuid + ext
	if err := knowledgeDAO.Upsert(&model.KnowledgeFile{Username: username, Filename: filename, Status: model.KnowledgePending}); err != nil {
		return "", fmt.Errorf("create knowledge file metadata: %w", err)
	}

	stagingDir := filepath.Join(userDir, ".staging")
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		log.Printf("Failed to create staging directory %s: %v", stagingDir, err)
		return "", err
	}
	filePath := filepath.Join(userDir, filename)
	stagingPath := filepath.Join(stagingDir, filename)

	if err := copyUpload(stagingPath, file); err != nil {
		log.Printf("Failed to copy uploaded file: %v", err)
		return "", err
	}
	removeStaging := true
	defer func() {
		if removeStaging {
			_ = os.Remove(stagingPath)
		}
	}()

	if err := knowledgeDAO.UpdateStatus(username, filename, model.KnowledgeIndexing, "", "", ""); err != nil {
		return "", fmt.Errorf("mark knowledge file indexing: %w", err)
	}
	indexer, err := rag.NewRAGIndexerForUser(username, filename)
	if err != nil {
		if statusErr := knowledgeDAO.UpdateStatus(username, filename, model.KnowledgeFailed, err.Error(), "", ""); statusErr != nil {
			log.Printf("mark failed knowledge file: %v", statusErr)
		}
		log.Printf("Failed to create RAG indexer: %v", err)
		return "", err
	}

	if err := indexer.IndexFile(context.Background(), stagingPath); err != nil {
		if statusErr := knowledgeDAO.UpdateStatus(username, filename, model.KnowledgeFailed, err.Error(), "", ""); statusErr != nil {
			log.Printf("mark failed knowledge file: %v", statusErr)
		}
		log.Printf("Failed to index file: %v", err)
		_ = rag.DeleteIndex(context.Background(), username, filename)
		return "", err
	}

	if err := os.Rename(stagingPath, filePath); err != nil {
		log.Printf("Failed to finalize uploaded file %s: %v", filePath, err)
		_ = rag.DeleteIndex(context.Background(), username, filename)
		return "", err
	}
	removeStaging = false
	if err := knowledgeDAO.UpdateStatus(username, filename, model.KnowledgeReady, "", indexer.Generation(), indexer.Fingerprint()); err != nil {
		return "", fmt.Errorf("mark knowledge file ready: %w", err)
	}

	log.Printf("File uploaded and indexed successfully: %s", filePath)
	return filePath, nil
}

type FileInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Status string `json:"status"`
}

func ListRagFiles(username string) ([]FileInfo, error) {
	userDir := filepath.Join(config.GetConfig().RuntimeConfig.UploadDir, username)
	entries, err := os.ReadDir(userDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	items := make([]FileInfo, 0)
	metadata, err := knowledgeDAO.List(username)
	if err != nil {
		return nil, fmt.Errorf("list knowledge file metadata: %w", err)
	}
	metadataByName := make(map[string]model.KnowledgeFile, len(metadata))
	for _, item := range metadata {
		metadataByName[item.Filename] = item
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == ".staging" || entry.Name()[0] == '.' {
			continue
		}
		seen[entry.Name()] = struct{}{}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		status := "pending"
		if item, ok := metadataByName[entry.Name()]; ok {
			status = item.Status
		} else {
			active, activeErr := rag.ActiveIndexStatus(context.Background(), username, entry.Name())
			if activeErr == nil && active {
				status = model.KnowledgeReady
			} else if activeErr == nil {
				status = model.KnowledgeStale
			}
		}
		items = append(items, FileInfo{Name: entry.Name(), Size: info.Size(), Status: status})
	}
	// Keep failed or in-progress records visible even when their source is
	// still staged or was removed after an indexing failure.
	for name, item := range metadataByName {
		if _, ok := seen[name]; ok {
			continue
		}
		items = append(items, FileInfo{Name: name, Status: item.Status})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func DeleteRagFile(username, filename string) error {
	if filename == "" || filepath.Base(filename) != filename {
		return fmt.Errorf("invalid file name")
	}
	// Remove the vector index first. If Redis is unavailable, keep the source
	// file and metadata so a retry can remove the complete knowledge item.
	if err := rag.DeleteIndex(context.Background(), username, filename); err != nil {
		return fmt.Errorf("delete RAG index: %w", err)
	}
	userDir := filepath.Join(config.GetConfig().RuntimeConfig.UploadDir, username)
	path := filepath.Join(userDir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := knowledgeDAO.Delete(username, filename); err != nil {
		return fmt.Errorf("delete knowledge file metadata: %w", err)
	}
	return nil
}

func copyUpload(destination string, file *multipart.FileHeader) error {
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}
	removeOnError := true
	defer func() {
		_ = dst.Close()
		if removeOnError {
			_ = os.Remove(destination)
		}
	}()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy upload: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close staging file: %w", err)
	}
	removeOnError = false
	return nil
}
