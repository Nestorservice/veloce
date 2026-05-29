package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type tokenData struct {
	FlashInputTokens  int64 `json:"flash_input_tokens"`
	FlashOutputTokens int64 `json:"flash_output_tokens"`
	ProInputTokens    int64 `json:"pro_input_tokens"`
	ProOutputTokens   int64 `json:"pro_output_tokens"`
	CachedTokensSaved int64 `json:"cached_tokens_saved"`
}

type TokenUsage struct {
	mu      sync.RWMutex
	rootDir string
	limit   int64
	data    tokenData
}

func NewTokenUsage(outputDir string, limit int) *TokenUsage {
	return &TokenUsage{rootDir: outputDir, limit: int64(limit)}
}

func LoadTokenUsage(outputDir string, limit int) (*TokenUsage, error) {
	path := filepath.Join(outputDir, ".veloce", "token_usage.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewTokenUsage(outputDir, limit), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read token_usage: %w", err)
	}
	var d tokenData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse token_usage: %w", err)
	}
	return &TokenUsage{rootDir: outputDir, limit: int64(limit), data: d}, nil
}

func (t *TokenUsage) AddFlash(in, out int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.FlashInputTokens += int64(in)
	t.data.FlashOutputTokens += int64(out)
}

func (t *TokenUsage) AddPro(in, out int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.ProInputTokens += int64(in)
	t.data.ProOutputTokens += int64(out)
}

func (t *TokenUsage) AddCachedSaved(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.CachedTokensSaved += int64(n)
}

func (t *TokenUsage) Total() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.data.FlashInputTokens + t.data.FlashOutputTokens +
		t.data.ProInputTokens + t.data.ProOutputTokens
}

func (t *TokenUsage) Exceeded() bool {
	return t.Total() >= t.limit
}

func (t *TokenUsage) Snapshot() (flashIn, flashOut, proIn, proOut int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.data.FlashInputTokens, t.data.FlashOutputTokens, t.data.ProInputTokens, t.data.ProOutputTokens
}

func (t *TokenUsage) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	dir := filepath.Join(t.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload := struct {
		tokenData
		Total     int64 `json:"total_tokens"`
		BudgetLim int64 `json:"budget_limit"`
	}{t.data, t.data.FlashInputTokens + t.data.FlashOutputTokens + t.data.ProInputTokens + t.data.ProOutputTokens, t.limit}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "token_usage.json.tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()
	return os.Rename(tmpPath, filepath.Join(dir, "token_usage.json"))
}
