package gemini

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheManager_CreatesAndPersistsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"name":"cachedContents/abc123"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cm := &CacheManager{
		baseURL: srv.URL,
		apiKey:  "k",
		http:    http.DefaultClient,
		rootDir: dir,
	}

	id, err := cm.EnsureCache("rules content", "types content")
	if err != nil {
		t.Fatalf("EnsureCache: %v", err)
	}
	if id != "cachedContents/abc123" {
		t.Errorf("id = %q", id)
	}
	stored, _ := os.ReadFile(filepath.Join(dir, ".veloce", "context_cache_id.txt"))
	if string(stored) != id {
		t.Errorf("persisted id = %q, want %q", stored, id)
	}
}
