package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrDailyQuotaExceeded signals a per-day quota hit. The free tier of
// OpenRouter resets daily; retries within the same day are pointless.
type ErrDailyQuotaExceeded struct{ Body string }

func (e *ErrDailyQuotaExceeded) Error() string {
	return "openrouter daily quota exhausted — wait until reset, switch key, or top up"
}

// ProgressFunc is an optional callback the HTTP client calls to report live
// status (sending request, rate-limited countdown, etc.). Implementations must
// be non-blocking — use a buffered channel or a no-wait select.
type ProgressFunc func(msg string)

// httpClient is the shared implementation for Worker and Architect clients.
// They differ only by name + model + (optionally) per-call defaults.
type httpClient struct {
	name    string // "worker" | "architect"
	model   string // e.g. "qwen/qwen3-coder:free"
	baseURL string
	apiKey  string
	http    *http.Client

	// Optional OpenRouter app-stats headers (sent if non-empty).
	httpReferer string
	xTitle      string

	limiter    *RateLimiter // shared across clients
	onProgress ProgressFunc // live status callback (may be nil)
}

// SetProgress wires a live-progress callback into the client.
func (c *httpClient) SetProgress(fn ProgressFunc) { c.onProgress = fn }

func (c *httpClient) notifyProgress(msg string) {
	if c.onProgress != nil {
		c.onProgress(msg)
	}
}

func (c *httpClient) Name() string  { return c.name }
func (c *httpClient) Model() string { return c.model }

// SetRateLimiter wires the shared limiter so the client throttles itself.
func (c *httpClient) SetRateLimiter(l *RateLimiter) { c.limiter = l }

// SetAppMetadata sets the optional HTTP-Referer and X-Title headers that
// OpenRouter uses for its app-stats leaderboard. Both are optional.
func (c *httpClient) SetAppMetadata(referer, title string) {
	c.httpReferer = referer
	c.xTitle = title
}

// --- wire types (OpenAI-compatible) -----------------------------------------

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type wireReq struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type wireChoice struct {
	Index        int         `json:"index"`
	Message      wireMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type wireResp struct {
	ID      string       `json:"id"`
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
	Error   *wireError   `json:"error,omitempty"`
}

type wireError struct {
	Message  string         `json:"message"`
	Code     any            `json:"code"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// --- main Complete ----------------------------------------------------------

func (c *httpClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Build messages: SystemRules (if any) → req.Messages → final user Prompt (if any).
	msgs := make([]wireMessage, 0, len(req.Messages)+2)
	if req.SystemRules != "" {
		msgs = append(msgs, wireMessage{Role: string(RoleSystem), Content: req.SystemRules})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, wireMessage{Role: string(m.Role), Content: m.Content})
	}
	if req.Prompt != "" {
		msgs = append(msgs, wireMessage{Role: string(RoleUser), Content: req.Prompt})
	}
	if len(msgs) == 0 {
		return nil, errors.New("openrouter: empty request (no system rules, no messages, no prompt)")
	}

	temp := req.Temperature
	if temp == 0 {
		temp = 0.2
	}
	body := wireReq{
		Model:       c.model,
		Messages:    msgs,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: temp,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openrouter marshal: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"

	const maxAttempts = 5
	var (
		raw    []byte
		status int
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}

		c.notifyProgress(fmt.Sprintf("sending request · attempt %d/%d", attempt, maxAttempts))

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		if c.httpReferer != "" {
			httpReq.Header.Set("HTTP-Referer", c.httpReferer)
		}
		if c.xTitle != "" {
			httpReq.Header.Set("X-Title", c.xTitle)
		}

		c.notifyProgress(fmt.Sprintf("waiting for response · attempt %d/%d", attempt, maxAttempts))
		resp, err := c.http.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("openrouter request: %w", err)
		}
		raw, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		status = resp.StatusCode

		if status == 429 || status == 503 || status == 502 {
			if isDailyQuota(raw) {
				return nil, &ErrDailyQuotaExceeded{Body: string(raw)}
			}
			delay := parseRetryDelay(resp.Header, raw)
			if delay == 0 {
				delay = 30 * time.Second
			} else {
				delay += 2 * time.Second // small buffer
			}
			if c.limiter != nil {
				c.limiter.Penalize(delay)
			}
			// Countdown with live progress updates every second.
			deadline := time.Now().Add(delay)
			for {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					break
				}
				secs := int(remaining.Seconds()) + 1
				c.notifyProgress(fmt.Sprintf("rate-limited · retry in %ds (attempt %d/%d)", secs, attempt, maxAttempts))
				step := time.Second
				if remaining < step {
					step = remaining
				}
				select {
				case <-time.After(step):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			continue
		}
		break
	}

	if status >= 300 {
		return nil, fmt.Errorf("openrouter %d: %s", status, string(raw))
	}

	var parsed wireResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("openrouter decode: %w (raw: %s)", err, truncate(string(raw), 200))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("openrouter API error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openrouter: empty choices (raw: %s)", truncate(string(raw), 200))
	}

	return &CompletionResponse{
		Text:         parsed.Choices[0].Message.Content,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
	}, nil
}

// --- helpers ----------------------------------------------------------------

var (
	retryAfterSecondsRe = regexp.MustCompile(`retry[\s_-]?after[^0-9]*(\d+)`)
	dailyQuotaRe        = regexp.MustCompile(`(?i)per[\s-]?day|daily|exceeded your.*daily`)
)

// parseRetryDelay reads Retry-After header first, then probes the body for
// common hints. Returns 0 if nothing can be parsed.
func parseRetryDelay(h http.Header, body []byte) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	if m := retryAfterSecondsRe.FindStringSubmatch(strings.ToLower(string(body))); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return 0
}

func isDailyQuota(body []byte) bool {
	return dailyQuotaRe.Match(body)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
