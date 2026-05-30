package batcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nestorservice/veloce/internal/scanner"
)

// FileWithContent pairs a scanner.File with the raw source read from disk.
type FileWithContent struct {
	scanner.File
	Content string
}

// Batch is a group of source files that will be sent to the LLM in a single
// request. All files in a batch share the same Phase and belong to the same
// logical module / directory cluster.
type Batch struct {
	ID          string            // human-readable label e.g. "phase2-batch3"
	Phase       int
	Files       []FileWithContent
	InputTokens int // estimated total input tokens for the batch
}

// Options controls how batches are formed.
type Options struct {
	// MaxFiles caps the number of files per batch (default DefaultMaxFiles).
	MaxFiles int
	// MaxInputTokens caps estimated input tokens per batch (default DefaultMaxInputTokens).
	MaxInputTokens int
}

func (o Options) maxFiles() int {
	if o.MaxFiles > 0 {
		return o.MaxFiles
	}
	return DefaultMaxFiles
}

func (o Options) maxTokens() int {
	if o.MaxInputTokens > 0 {
		return o.MaxInputTokens
	}
	return DefaultMaxInputTokens
}

// Group reads each file from disk and groups them into batches.
// Files within the same phase are further clustered by their first two
// directory levels (e.g. app/Http/Controllers/Admin) so that related
// controllers/models travel together and the LLM can resolve intra-module
// references.
//
// Oversized files (> MaxInputTokens on their own) are placed in a singleton
// batch so they always get processed, just individually.
func Group(root string, files []scanner.File, opts Options) ([]Batch, error) {
	maxF := opts.maxFiles()
	maxT := opts.maxTokens()

	// 1. Read all files; skip unreadable ones with a warning.
	withContent := make([]FileWithContent, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f.AbsPath)
		if err != nil {
			fmt.Printf("  batcher: skip unreadable %s: %v\n", f.RelPath, err)
			continue
		}
		withContent = append(withContent, FileWithContent{File: f, Content: string(raw)})
	}

	// 2. Group by (phase, clusterKey).
	type clusterID struct {
		phase int
		key   string
	}
	order := []clusterID{}
	clusters := map[clusterID][]FileWithContent{}

	for _, f := range withContent {
		key := clusterKey(f.RelPath)
		id := clusterID{f.Phase, key}
		if _, exists := clusters[id]; !exists {
			order = append(order, id)
		}
		clusters[id] = append(clusters[id], f)
	}

	// 3. Split each cluster into batches respecting maxFiles and maxTokens.
	var batches []Batch
	batchNum := 0

	for _, id := range order {
		group := clusters[id]
		var current []FileWithContent
		currentTokens := 0

		flush := func() {
			if len(current) == 0 {
				return
			}
			batchNum++
			batches = append(batches, Batch{
				ID:          fmt.Sprintf("p%d-b%d", id.phase, batchNum),
				Phase:       id.phase,
				Files:       current,
				InputTokens: currentTokens,
			})
			current = nil
			currentTokens = 0
		}

		for _, f := range group {
			tok := EstimateTokens(f.Content)

			// Singleton: file alone exceeds token limit.
			if tok > maxT {
				flush()
				batchNum++
				batches = append(batches, Batch{
					ID:          fmt.Sprintf("p%d-b%d-big", id.phase, batchNum),
					Phase:       id.phase,
					Files:       []FileWithContent{f},
					InputTokens: tok,
				})
				continue
			}

			// Would overflow? Flush current batch first.
			if len(current) >= maxF || (currentTokens+tok > maxT && len(current) > 0) {
				flush()
			}

			current = append(current, f)
			currentTokens += tok
		}
		flush()
	}

	_ = root // reserved for future use (e.g. reading .env from root)
	return batches, nil
}

// clusterKey returns the first two path segments of rel, normalized to
// forward slashes. This groups files into logical module clusters.
//
//	"app/Http/Controllers/Admin/Foo.php"  → "app/Http"
//	"Modules/Fees/Http/Controllers/X.php" → "Modules/Fees"
//	"config/app.php"                      → "config"
func clusterKey(rel string) string {
	rel = filepath.ToSlash(rel)
	parts := strings.SplitN(rel, "/", 4)
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0]
	default:
		// For Modules/<Name>/... use the first two parts (Modules/Name).
		if parts[0] == "Modules" && len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return parts[0] + "/" + parts[1]
	}
}
