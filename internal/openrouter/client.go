// Package openrouter provides an OpenAI-compatible client targeted at
// https://openrouter.ai. Two model classes are exposed:
//
//   - Worker  : the heavy-lifting code translator (DeepSeek V4 Flash, 1M ctx).
//   - Architect: the planner / batch organizer (Llama 3.3 70B Instruct).
//
// Both implement the same Client interface so they are interchangeable for the
// pipeline + corrector code in internal/pipeline and internal/agent.
package openrouter

import "context"

// Role identifies the persona of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in the conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest is the abstract request shared by all OpenRouter clients.
// SystemRules is hoisted to a leading system message; Messages may be empty
// when a single user Prompt is enough — Prompt is appended as the final user
// message and is the simplest path for callers.
type CompletionRequest struct {
	SystemRules     string    // immutable rules (sent as system message)
	Messages        []Message // optional multi-turn history
	Prompt          string    // convenience: appended as final user message
	MaxOutputTokens int
	Temperature     float64 // 0 falls back to 0.2
}

// CompletionResponse is the abstract response.
type CompletionResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
}

// Client is implemented by both the Worker (DeepSeek) and Architect (Llama).
type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Name() string // "worker" or "architect"
	Model() string
}
