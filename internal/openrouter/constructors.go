package openrouter

import (
	"net/http"
	"time"
)

// Base URLs
const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	groqBaseURL    = "https://api.groq.com/openai/v1"
)

// ---- OpenRouter (default) --------------------------------------------------

// Default free-tier model identifiers on OpenRouter.
const (
	DefaultWorkerModel    = "qwen/qwen3-coder:free"
	DefaultArchitectModel = "meta-llama/llama-3.3-70b-instruct:free"
)

// FallbackWorkerModels are tried in order when the primary worker model is
// rate-limited (429) or has no endpoints (404) from its upstream provider.
// openrouter/free is the last resort — it auto-routes to whatever is available.
var FallbackWorkerModels = []string{
	"deepseek/deepseek-r1:free",       // DeepSeek R1 — reasoning + code
	"deepseek/deepseek-v4-flash:free", // DeepSeek V4 Flash — 1M ctx, fast
	"deepseek/deepseek-chat:free",     // DeepSeek V3 — stable
	"google/gemma-4-31b-it:free",      // Google Gemma 4 31B — different infra
	"openrouter/free",                 // auto-router: picks whatever is up right now
}

// NewWorkerClient returns the primary code translator using OpenRouter.
func NewWorkerClient(apiKey string) Client {
	return newHTTPWithBase("worker", DefaultWorkerModel, apiKey, defaultBaseURL)
}

// NewArchitectClient returns the planner using OpenRouter.
func NewArchitectClient(apiKey string) Client {
	return newHTTPWithBase("architect", DefaultArchitectModel, apiKey, defaultBaseURL)
}

// NewWorkerClientWithModel lets callers pin a specific OpenRouter worker model.
func NewWorkerClientWithModel(apiKey, model string) Client {
	return newHTTPWithBase("worker", model, apiKey, defaultBaseURL)
}

// ---- Groq ------------------------------------------------------------------

// Groq models ordered by free-tier TPM (tokens/min) — highest first.
// llama-3.1-8b-instant has the highest TPM on the free tier (~30K TPM).
// qwen/qwen3-32b has only 6K TPM — used as fallback for smaller requests.
const (
	DefaultGroqWorkerModel    = "llama-3.1-8b-instant"
	DefaultGroqArchitectModel = "llama-3.3-70b-versatile"
)

// GroqFallbackWorkerModels are tried in order when the primary Groq model fails.
var GroqFallbackWorkerModels = []string{
	"llama-3.3-70b-versatile",                   // Llama 3.3 70B — more capable, ~12K TPM
	"openai/gpt-oss-20b",                        // GPT OSS 20B — fast
	"qwen/qwen3-32b",                            // Qwen 3 32B — reasoning (6K TPM, low)
	"meta-llama/llama-4-scout-17b-16e-instruct", // Llama 4 Scout — last resort
}

// NewGroqWorkerClient returns the primary code translator using Groq.
func NewGroqWorkerClient(apiKey string) Client {
	return newHTTPWithBase("worker", DefaultGroqWorkerModel, apiKey, groqBaseURL)
}

// NewGroqArchitectClient returns the planner using Groq.
func NewGroqArchitectClient(apiKey string) Client {
	return newHTTPWithBase("architect", DefaultGroqArchitectModel, apiKey, groqBaseURL)
}

// NewGroqWorkerClientWithModel lets callers pin a specific Groq worker model.
func NewGroqWorkerClientWithModel(apiKey, model string) Client {
	return newHTTPWithBase("worker", model, apiKey, groqBaseURL)
}

// ---- shared constructors ---------------------------------------------------

func newHTTPWithBase(name, model, apiKey, baseURL string) *httpClient {
	return &httpClient{
		name:    name,
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// newHTTP keeps backward compat with the old zero-arg base URL form.
func newHTTP(name, model, apiKey string) *httpClient {
	return newHTTPWithBase(name, model, apiKey, defaultBaseURL)
}

// AttachLimiter wires a shared rate limiter into a client.
func AttachLimiter(c Client, l *RateLimiter) {
	if h, ok := c.(*httpClient); ok {
		h.SetRateLimiter(l)
	}
}

// AttachAppMetadata sets the HTTP-Referer and X-Title headers (OpenRouter only).
func AttachAppMetadata(c Client, referer, title string) {
	if h, ok := c.(*httpClient); ok {
		h.SetAppMetadata(referer, title)
	}
}

// AttachProgress wires a live-status callback into a client.
func AttachProgress(c Client, fn ProgressFunc) {
	if h, ok := c.(*httpClient); ok {
		h.SetProgress(fn)
	}
}
