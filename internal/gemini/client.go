package gemini

import "context"

// CompletionRequest is the abstract request passed to a Gemini client.
type CompletionRequest struct {
	SystemRules     string // immutable rules (cached)
	CachedID        string // context cache id (empty = no cache)
	Prompt          string // user-visible prompt
	MaxOutputTokens int
}

// CompletionResponse is the abstract response.
type CompletionResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
	CachedTokens int // tokens served from cache (reported by API)
}

// Client is the contract implemented by both Flash and Pro clients.
type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Name() string // "flash" or "pro"
}
