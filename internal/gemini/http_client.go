package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type httpClient struct {
	name    string // "flash" | "pro"
	model   string // "gemini-2.5-flash" | "gemini-2.5-pro"
	baseURL string // e.g. "https://generativelanguage.googleapis.com/v1beta"
	apiKey  string
	http    *http.Client
	limiter *RateLimiter // shared across flash + pro
}

// SetRateLimiter wires the shared limiter so the client throttles itself.
func (c *httpClient) SetRateLimiter(l *RateLimiter) { c.limiter = l }

var retryDelayRe = regexp.MustCompile(`"retryDelay"\s*:\s*"(\d+)s"`)

func parseRetryDelay(body string) time.Duration {
	m := retryDelayRe.FindStringSubmatch(body)
	if len(m) != 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return time.Duration(n) * time.Second
}

func (c *httpClient) Name() string { return c.name }

type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiReqBody struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	CachedContent     string          `json:"cachedContent,omitempty"`
	GenerationConfig  map[string]any  `json:"generationConfig,omitempty"`
}
type geminiCandidate struct {
	Content geminiContent `json:"content"`
}
type geminiUsage struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}
type geminiResp struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
}

func (c *httpClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	body := geminiReqBody{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}}},
		GenerationConfig: map[string]any{
			"responseMimeType": "text/plain",
			"temperature":      0.2,
		},
	}
	if req.SystemRules != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.SystemRules}}}
	}
	if req.CachedID != "" {
		body.CachedContent = req.CachedID
	}
	if req.MaxOutputTokens > 0 {
		body.GenerationConfig["maxOutputTokens"] = req.MaxOutputTokens
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

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
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("gemini request: %w", err)
		}
		raw, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		status = resp.StatusCode

		if status == 429 || status == 503 {
			delay := parseRetryDelay(string(raw))
			if delay == 0 {
				delay = time.Duration(1<<attempt) * time.Second
			} else {
				delay += 2 * time.Second // small buffer
			}
			if c.limiter != nil {
				c.limiter.Penalize(delay)
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}
		break
	}
	if status >= 300 {
		return nil, fmt.Errorf("gemini %d: %s", status, string(raw))
	}
	var parsed geminiResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini: empty response")
	}
	return &CompletionResponse{
		Text:         parsed.Candidates[0].Content.Parts[0].Text,
		InputTokens:  parsed.UsageMetadata.PromptTokenCount,
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		CachedTokens: parsed.UsageMetadata.CachedContentTokenCount,
	}, nil
}
