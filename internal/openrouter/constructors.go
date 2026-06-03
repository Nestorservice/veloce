package openrouter

import (
	"net/http"
	"time"
)

// Default free-tier model identifiers on OpenRouter.
//
// Override with NewWorkerClientWithModel / NewArchitectClientWithModel if the
// :free suffix or the slug changes (OpenRouter periodically renames variants).
const (
	DefaultWorkerModel    = "qwen/qwen3-coder:free"
	DefaultArchitectModel = "meta-llama/llama-3.3-70b-instruct:free"
)

// NewWorkerClient returns the heavy translator (Qwen3 Coder 480B, 1M ctx, 262K output).
// Used for batched PHP→Go/Dart conversion in the worker pool.
func NewWorkerClient(apiKey string) Client {
	return newHTTP("worker", DefaultWorkerModel, apiKey)
}

// NewArchitectClient returns the planner (Llama 3.3 70B Instruct).
// Used for arborescence analysis, batch design, escalation on stubborn files.
func NewArchitectClient(apiKey string) Client {
	return newHTTP("architect", DefaultArchitectModel, apiKey)
}

// NewWorkerClientWithModel lets callers pin a specific worker model
// (e.g. a paid tier without the :free suffix).
func NewWorkerClientWithModel(apiKey, model string) Client {
	return newHTTP("worker", model, apiKey)
}

// NewArchitectClientWithModel lets callers pin a specific architect model.
func NewArchitectClientWithModel(apiKey, model string) Client {
	return newHTTP("architect", model, apiKey)
}

func newHTTP(name, model, apiKey string) *httpClient {
	return &httpClient{
		name:    name,
		model:   model,
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// AttachLimiter wires a shared rate limiter into a client returned by any of
// the constructors above. No-op if the client is not the built-in HTTP impl.
func AttachLimiter(c Client, l *RateLimiter) {
	if h, ok := c.(*httpClient); ok {
		h.SetRateLimiter(l)
	}
}

// AttachAppMetadata sets the OpenRouter app-stats headers (HTTP-Referer + X-Title).
// Both are optional but recommended for visibility in the OpenRouter dashboard.
func AttachAppMetadata(c Client, referer, title string) {
	if h, ok := c.(*httpClient); ok {
		h.SetAppMetadata(referer, title)
	}
}

// AttachProgress wires a live-status callback into a client. The callback is
// called from within Complete() to report per-attempt status and rate-limit
// countdown messages. Must be non-blocking.
func AttachProgress(c Client, fn ProgressFunc) {
	if h, ok := c.(*httpClient); ok {
		h.SetProgress(fn)
	}
}
