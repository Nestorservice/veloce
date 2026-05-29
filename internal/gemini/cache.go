package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type CacheManager struct {
	baseURL string
	apiKey  string
	http    *http.Client
	rootDir string // output dir; cache id stored under .veloce/
}

func NewCacheManager(apiKey, rootDir string) *CacheManager {
	return &CacheManager{
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		http:    &http.Client{},
		rootDir: rootDir,
	}
}

// EnsureCache creates a new cached content blob and persists its ID.
// Returns the cache ID to be passed as `cached_content` in subsequent requests.
func (m *CacheManager) EnsureCache(rules, types string) (string, error) {
	combined := rules + "\n\n" + types
	body := map[string]any{
		"model": "models/gemini-2.5-flash",
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": combined}}},
		},
		"ttl": "3600s",
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/cachedContents?key=%s", m.baseURL, m.apiKey)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("cache request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("cache create %d: %s", resp.StatusCode, raw)
	}
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("cache decode: %w", err)
	}
	dir := filepath.Join(m.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "context_cache_id.txt"), []byte(parsed.Name), 0o644); err != nil {
		return "", err
	}
	return parsed.Name, nil
}

// LoadCacheID returns the persisted cache id if present.
func (m *CacheManager) LoadCacheID() (string, error) {
	b, err := os.ReadFile(filepath.Join(m.rootDir, ".veloce", "context_cache_id.txt"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
