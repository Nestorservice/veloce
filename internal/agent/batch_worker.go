package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nestorservice/veloce/internal/batcher"
	"github.com/Nestorservice/veloce/internal/openrouter"
	"github.com/Nestorservice/veloce/internal/state"
)

// BatchWorker processes one batcher.Batch per call. It sends all files in the
// batch to the Worker LLM (DeepSeek) in a single request, parses the
// multi-file response, writes output files, and updates migration state.
//
// On daily quota exhaustion, it returns ErrDailyQuota so the orchestrator can
// stop immediately and advise the user.
//
// On a partial response (some source files have no matching output), the
// missing files are marked Failed so they can be retried individually on the
// next run.
type BatchWorker struct {
	OutputRoot  string
	Worker      openrouter.Client // DeepSeek V4 Flash
	Architect   openrouter.Client // Llama 3.3 – used only for escalation
	Migration   *state.MigrationState
	TokenUsage  *state.TokenUsage
	SystemRules string
	MaxRetries  int // per-batch attempt ceiling (default 3)
}

// ErrDailyQuota is returned when the API signals a per-day quota exhaustion
// that cannot be resolved by waiting within the current session.
var ErrDailyQuota = errors.New("daily API quota exhausted")

// Process translates all files in the batch and writes output. It is safe to
// call from multiple goroutines (each Batch is independent).
func (bw *BatchWorker) Process(ctx context.Context, b batcher.Batch) error {
	maxRetries := bw.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// Mark all files as processing.
	for _, f := range b.Files {
		bw.Migration.Mark(f.RelPath, state.FileEntry{
			Status: state.StatusProcessing,
			Phase:  f.Phase,
		})
	}

	prompt := openrouter.BuildBatchPrompt(b)
	req := openrouter.CompletionRequest{
		SystemRules:     bw.SystemRules,
		Prompt:          prompt,
		MaxOutputTokens: 8192 * len(b.Files), // generous ceiling per file
	}

	var (
		parsed []openrouter.ParsedFile
		resp   *openrouter.CompletionResponse
		lastErr error
	)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// First attempt: Worker; on failure escalate to Architect.
		client := bw.Worker
		if attempt > 1 {
			client = bw.Architect
		}

		resp, lastErr = client.Complete(ctx, req)
		if lastErr != nil {
			var daily *openrouter.ErrDailyQuotaExceeded
			if errors.As(lastErr, &daily) {
				bw.markAllFailed(b, lastErr.Error())
				return ErrDailyQuota
			}
			// Transient error — wait briefly then retry.
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		// Track token usage (Worker = Flash equivalent; Architect = Pro equivalent).
		if attempt == 1 {
			bw.TokenUsage.AddFlash(resp.InputTokens, resp.OutputTokens)
		} else {
			bw.TokenUsage.AddPro(resp.InputTokens, resp.OutputTokens)
		}

		// Parse the multi-file response.
		parsed = openrouter.ParseBatchResponse(resp.Text)
		if len(parsed) > 0 {
			break
		}
		// Empty parse on first attempt → retry with Architect.
		lastErr = fmt.Errorf("model returned no parseable output files (attempt %d)", attempt)
	}

	if len(parsed) == 0 {
		bw.markAllFailed(b, fmt.Sprintf("no output after %d attempts: %v", maxRetries, lastErr))
		return lastErr
	}

	// Write output files.
	writtenPaths := map[string]bool{}
	for _, pf := range parsed {
		outPath := filepath.Join(bw.OutputRoot, filepath.FromSlash(pf.Path))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
		}
		if err := os.WriteFile(outPath, []byte(pf.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		writtenPaths[pf.Path] = true
	}

	// Update migration state for each source file.
	for _, f := range b.Files {
		// Try to find the matching output path heuristically.
		outputPath := matchOutput(f.RelPath, parsed)
		entry := state.FileEntry{
			Phase:  f.Phase,
			Output: outputPath,
		}
		if outputPath == "" {
			entry.Status = state.StatusFailed
			entry.LastError = "no output generated for this file"
		} else {
			entry.Status = state.StatusDone
		}
		bw.Migration.Mark(f.RelPath, entry)
	}

	return nil
}

// --- helpers ----------------------------------------------------------------

func (bw *BatchWorker) markAllFailed(b batcher.Batch, reason string) {
	for _, f := range b.Files {
		bw.Migration.Mark(f.RelPath, state.FileEntry{
			Status:    state.StatusFailed,
			Phase:     f.Phase,
			LastError: reason,
		})
	}
}

// matchOutput tries to find the output path in parsed that corresponds to the
// given source rel path, using a best-effort heuristic (stem match).
func matchOutput(srcRel string, parsed []openrouter.ParsedFile) string {
	// Exact stem match: strip extension and compare base names.
	srcBase := strings.ToLower(stemNoExt(srcRel))

	for _, pf := range parsed {
		outBase := strings.ToLower(stemNoExt(pf.Path))
		// Direct stem match.
		if outBase == srcBase {
			return pf.Path
		}
		// Suffix match handles e.g. "product_controller" matching
		// "ProductController" → "product_controller_handler".
		if strings.HasPrefix(outBase, srcBase) || strings.HasSuffix(outBase, srcBase) {
			return pf.Path
		}
	}

	// Fallback: if only one output file, map everything to it (single-file batch).
	if len(parsed) == 1 {
		return parsed[0].Path
	}
	return ""
}

func stemNoExt(p string) string {
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	s := strings.TrimSuffix(base, ext)
	// Strip common suffixes added by prompts.go (e.g. "_handler", "_request").
	for _, suffix := range []string{"_handler", "_request", "_screen", "_controller"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}
