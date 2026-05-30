package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer returns an *httptest.Server that mimics OpenRouter's
// /v1/chat/completions endpoint. The hook lets each test override behavior.
func fakeServer(t *testing.T, hook func(req wireReq, attempt int) (int, any)) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		body, _ := io.ReadAll(r.Body)
		var parsed wireReq
		_ = json.Unmarshal(body, &parsed)
		attempt := int(atomic.AddInt32(&calls, 1))
		status, payload := hook(parsed, attempt)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newClient(t *testing.T, baseURL string) *httpClient {
	t.Helper()
	c := newHTTP("worker", "test-model", "sk-test")
	c.baseURL = baseURL
	c.http.Timeout = 5 * time.Second
	return c
}

func TestComplete_HappyPath(t *testing.T) {
	srv, calls := fakeServer(t, func(req wireReq, _ int) (int, any) {
		if req.Model != "test-model" {
			t.Errorf("model=%q, want test-model", req.Model)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("messages=%d, want 2 (system + user)", len(req.Messages))
		}
		if req.Messages[0].Role != "system" || req.Messages[0].Content != "RULES" {
			t.Errorf("system msg = %+v", req.Messages[0])
		}
		if req.Messages[1].Role != "user" || req.Messages[1].Content != "translate this" {
			t.Errorf("user msg = %+v", req.Messages[1])
		}
		return 200, wireResp{
			Choices: []wireChoice{{Message: wireMessage{Role: "assistant", Content: "package main"}}},
			Usage:   wireUsage{PromptTokens: 12, CompletionTokens: 7},
		}
	})
	c := newClient(t, srv.URL)
	resp, err := c.Complete(context.Background(), CompletionRequest{
		SystemRules: "RULES",
		Prompt:      "translate this",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "package main" {
		t.Errorf("text=%q", resp.Text)
	}
	if resp.InputTokens != 12 || resp.OutputTokens != 7 {
		t.Errorf("tokens in=%d out=%d", resp.InputTokens, resp.OutputTokens)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Errorf("calls=%d, want 1", *calls)
	}
}

func TestComplete_RetryOn429_HonorsRetryAfter(t *testing.T) {
	srv, calls := fakeServer(t, func(_ wireReq, attempt int) (int, any) {
		if attempt == 1 {
			return 429, map[string]any{"error": map[string]any{"message": "Too many requests"}}
		}
		return 200, wireResp{
			Choices: []wireChoice{{Message: wireMessage{Content: "ok"}}},
			Usage:   wireUsage{PromptTokens: 1, CompletionTokens: 1},
		}
	})
	c := newClient(t, srv.URL)
	// Replace the default 30s retry wait with something a test can tolerate by
	// hand-rolling: we instead patch the test by setting a header through a
	// custom transport? Simpler: trust the body parser to find nothing and
	// fall back to 30s -> too slow. So we inject Retry-After=1 via override:
	c.http.Transport = retryAfterTransport{wrapped: http.DefaultTransport, seconds: "1"}
	start := time.Now()
	resp, err := c.Complete(context.Background(), CompletionRequest{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "ok" {
		t.Errorf("text=%q", resp.Text)
	}
	if atomic.LoadInt32(calls) != 2 {
		t.Errorf("calls=%d, want 2 (1 fail + 1 retry)", *calls)
	}
	if dur := time.Since(start); dur < time.Second || dur > 10*time.Second {
		t.Errorf("retry duration=%s, want ~1-3s", dur)
	}
}

// retryAfterTransport injects a Retry-After header on responses that don't have one,
// so 429 tests can finish quickly without depending on body parsing.
type retryAfterTransport struct {
	wrapped http.RoundTripper
	seconds string
}

func (t retryAfterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.wrapped.RoundTrip(req)
	if err == nil && resp != nil && resp.StatusCode == 429 && resp.Header.Get("Retry-After") == "" {
		resp.Header.Set("Retry-After", t.seconds)
	}
	return resp, err
}

func TestComplete_DailyQuotaShortCircuits(t *testing.T) {
	srv, calls := fakeServer(t, func(_ wireReq, _ int) (int, any) {
		return 429, map[string]any{"error": map[string]any{
			"message": "You exceeded your daily free-tier quota, try again tomorrow",
		}}
	})
	c := newClient(t, srv.URL)
	_, err := c.Complete(context.Background(), CompletionRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("expected ErrDailyQuotaExceeded")
	}
	var daily *ErrDailyQuotaExceeded
	if !errorsAs(err, &daily) {
		t.Errorf("err type=%T (%v), want *ErrDailyQuotaExceeded", err, err)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Errorf("calls=%d, want 1 (no retry on daily)", *calls)
	}
}

func TestComplete_BadJSONErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	c := newClient(t, srv.URL)
	_, err := c.Complete(context.Background(), CompletionRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("err=%v", err)
	}
}

func TestComplete_EmptyRequestErrors(t *testing.T) {
	c := newClient(t, "http://unused")
	_, err := c.Complete(context.Background(), CompletionRequest{})
	if err == nil {
		t.Fatal("expected error for empty request")
	}
}

func TestRateLimiter_EnforcesMinInterval(t *testing.T) {
	l := NewRateLimiter(0, 50*time.Millisecond)
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	dur := time.Since(start)
	if dur < 100*time.Millisecond {
		t.Errorf("3 waits = %s, want >= 100ms (50ms × 2 gaps)", dur)
	}
}

func TestRateLimiter_PenalizePushesNextSlot(t *testing.T) {
	l := NewRateLimiter(0, 0)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	l.Penalize(80 * time.Millisecond)
	start := time.Now()
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if dur := time.Since(start); dur < 50*time.Millisecond {
		t.Errorf("post-penalty wait = %s, want >= 50ms", dur)
	}
}

// errorsAs is a tiny local helper so the test file doesn't need to import errors.
func errorsAs(err error, target any) bool {
	type targeter interface{ As(any) bool }
	_ = targeter(nil)
	// stdlib errors.As without the import indirection
	for {
		if err == nil {
			return false
		}
		if e, ok := err.(*ErrDailyQuotaExceeded); ok {
			if t, ok := target.(**ErrDailyQuotaExceeded); ok {
				*t = e
				return true
			}
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
}
