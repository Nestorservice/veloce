// Package batcher groups scanner.File lists into batches suitable for a
// single LLM call. DeepSeek V4 Flash supports ~1 M tokens of context; we
// leave half for the output and cap each batch at MaxInputTokens.
package batcher

// EstimateTokens approximates the token count of text using a chars/3.5
// heuristic calibrated against dense PHP source code. The estimate is
// intentionally conservative (rounds up) so batches stay safely under the
// model's context window.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	t := len(text) * 10 / 35 // ≈ len / 3.5 using integer arithmetic
	if t == 0 {
		return 1
	}
	return t
}

// DefaultMaxInputTokens is the per-batch ceiling for input tokens.
// DeepSeek V4 Flash has a 1 M token context window; we allocate 500 K for
// input and reserve the other 500 K for the generated output across all files
// in the batch.
const DefaultMaxInputTokens = 500_000

// DefaultMaxFiles is the maximum number of source files in a single batch.
// Keeping batches to ~15 files makes them easy to reason about and keeps
// individual prompts from becoming unwieldy even on smaller context models.
const DefaultMaxFiles = 15
