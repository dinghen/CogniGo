package rag

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
)

// EvalRecord is one JSONL retrieval evaluation case. DocumentSet is optional;
// callers may supply a fixed corpus separately when records only contain labels.
type EvalRecord struct {
	Query           string         `json:"query"`
	RelevantSources []string       `json:"relevant_sources"`
	DocumentSet     []EvalDocument `json:"document_set,omitempty"`
}

type EvalDocument struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

type EvalResult struct {
	Cases          int     `json:"cases"`
	K              int     `json:"k"`
	RecallAtK      float64 `json:"recall_at_k"`
	MRRAtK         float64 `json:"mrr_at_k"`
	P50Millis      float64 `json:"p50_ms"`
	P95Millis      float64 `json:"p95_ms"`
	RetrievalCalls int     `json:"retrieval_calls"`
	EmbeddingCalls int     `json:"embedding_calls"`
	Errors         int     `json:"errors"`
}

// EvaluateJSONL evaluates a deterministic local retriever. It is intentionally
// independent from Redis and provider credentials, so it can run in CI and is
// suitable for comparing chunking changes without claiming production metrics.
func EvaluateJSONL(r io.Reader, k int) (EvalResult, error) {
	if k <= 0 {
		k = 5
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	result := EvalResult{K: k}
	latencies := make([]float64, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record EvalRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			result.Errors++
			continue
		}
		if strings.TrimSpace(record.Query) == "" || len(record.DocumentSet) == 0 {
			result.Errors++
			continue
		}
		started := time.Now()
		// The default evaluator is deliberately lexical and does not call an
		// embedding provider. Keep that distinction explicit instead of
		// reporting a synthetic embedding-call count.
		result.RetrievalCalls++
		scored := rankDocuments(record.Query, record.DocumentSet)
		latencies = append(latencies, float64(time.Since(started).Microseconds())/1000)
		result.Cases++
		relevant := make(map[string]struct{}, len(record.RelevantSources))
		for _, source := range record.RelevantSources {
			relevant[source] = struct{}{}
		}
		hits, reciprocal := 0, 0.0
		for i, document := range scored {
			if i >= k {
				break
			}
			if _, ok := relevant[document.Source]; !ok {
				continue
			}
			hits++
			if reciprocal == 0 {
				reciprocal = 1 / float64(i+1)
			}
		}
		if len(relevant) > 0 {
			result.RecallAtK += float64(hits) / float64(len(relevant))
		}
		result.MRRAtK += reciprocal
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read evaluation JSONL: %w", err)
	}
	if result.Cases > 0 {
		result.RecallAtK /= float64(result.Cases)
		result.MRRAtK /= float64(result.Cases)
	}
	if len(latencies) > 0 {
		sort.Float64s(latencies)
		result.P50Millis = percentile(latencies, 0.50)
		result.P95Millis = percentile(latencies, 0.95)
	}
	return result, nil
}

type scoredDocument struct {
	EvalDocument
	score int
}

func rankDocuments(query string, documents []EvalDocument) []EvalDocument {
	queryTokens := tokenSet(query)
	scored := make([]scoredDocument, 0, len(documents))
	for _, document := range documents {
		score := 0
		for token := range queryTokens {
			if strings.Contains(strings.ToLower(document.Content), token) {
				score++
			}
		}
		scored = append(scored, scoredDocument{EvalDocument: document, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].Source < scored[j].Source
	})
	result := make([]EvalDocument, len(scored))
	for i := range scored {
		result[i] = scored[i].EvalDocument
	}
	return result
}

func tokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	var word []rune
	flush := func() {
		if len(word) > 0 {
			result[strings.ToLower(string(word))] = struct{}{}
			word = word[:0]
		}
	}
	for _, char := range []rune(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			word = append(word, char)
		} else {
			flush()
		}
	}
	flush()
	return result
}

func percentile(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * ratio)
	return values[index]
}
