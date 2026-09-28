package file

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/dinghen/CogniGo/common/rag"
	"github.com/dinghen/CogniGo/config"
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
	entries, err := os.ReadDir(userDir)
	if err != nil {
		log.Printf("Failed to inspect user directory %s: %v", userDir, err)
		return "", err
	}

	// Generate UUID as the stored filename. The original client filename is
	// used only for its validated extension, so it cannot escape userDir.
	uuid := utils.GenerateUUID()
	ext := filepath.Ext(file.Filename)
	filename := uuid + ext

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

	indexer, err := rag.NewRAGIndexer(username, filename, config.GetConfig().RagModelConfig.RagEmbeddingModel)
	if err != nil {
		log.Printf("Failed to create RAG indexer: %v", err)
		return "", err
	}

	if err := indexer.IndexFile(context.Background(), stagingPath); err != nil {
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

	// The new file is now valid. Remove the previous direct files and their
	// indexes; cleanup failures are logged so a successful upload is not
	// reported as failed because an old index was already absent.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		oldFilename := entry.Name()
		if err := rag.DeleteIndex(context.Background(), username, oldFilename); err != nil {
			log.Printf("Failed to delete index for %s: %v", oldFilename, err)
		}
		if err := os.Remove(filepath.Join(userDir, oldFilename)); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to remove old file %s: %v", oldFilename, err)
		}
	}

	log.Printf("File uploaded and indexed successfully: %s", filePath)
	return filePath, nil
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
