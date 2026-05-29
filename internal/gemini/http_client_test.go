package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClient_Complete_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "translate me") {
			t.Errorf("prompt missing in body: %s", body)
		}
		resp := map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "package domain"}}}},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":        120,
				"candidatesTokenCount":    30,
				"cachedContentTokenCount": 80,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &httpClient{
		name:    "flash",
		model:   "gemini-2.5-flash",
		baseURL: srv.URL,
		apiKey:  "fake",
		http:    http.DefaultClient,
	}

	resp, err := c.Complete(context.Background(), CompletionRequest{Prompt: "translate me"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "package domain" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.InputTokens != 120 || resp.OutputTokens != 30 || resp.CachedTokens != 80 {
		t.Errorf("token counts = (%d,%d,%d)", resp.InputTokens, resp.OutputTokens, resp.CachedTokens)
	}
}
