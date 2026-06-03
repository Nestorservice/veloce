package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Nestorservice/veloce/internal/agent"
	"github.com/Nestorservice/veloce/internal/batcher"
	"github.com/Nestorservice/veloce/internal/config"
	"github.com/Nestorservice/veloce/internal/openrouter"
	"github.com/Nestorservice/veloce/internal/scanner"
	"github.com/Nestorservice/veloce/internal/state"
)

var migrateFlags config.Flags

func phaseBreakdown(files []scanner.File) string {
	counts := map[int]int{}
	for _, f := range files {
		counts[f.Phase]++
	}
	return fmt.Sprintf("P1=%d P2=%d P3=%d P4=%d", counts[1], counts[2], counts[3], counts[4])
}

func registerMigrateFlags(flags *pflag.FlagSet) {
	flags.StringVar(&migrateFlags.Source, "source", "", "Path to Laravel project (default: current directory)")
	flags.StringVar(&migrateFlags.Output, "output", "", "Output path (default: <project>_output next to source)")
	flags.IntVar(&migrateFlags.Workers, "workers", 0, "Parallel batch workers (default: 3)")
	flags.StringVar(&migrateFlags.APIKey, "api-key", "", "OpenRouter API key (or $OPENROUTER_API_KEY)")
	flags.BoolVar(&migrateFlags.Resume, "resume", true, "Resume from last checkpoint if present")
	flags.BoolVar(&migrateFlags.DryRun, "dry-run", false, "Scan only — no API calls, no writes")
	flags.BoolVar(&migrateFlags.RunTests, "run-tests", false, "Run go test / flutter test after each batch")
	flags.IntVar(&migrateFlags.RPM, "rpm", 0, "Max API requests/min (default: 10 — OpenRouter free tier)")
	flags.IntVar(&migrateFlags.Delay, "delay", 0, "Forced pause in seconds between batches (default: 10)")
	flags.IntVar(&migrateFlags.BatchSize, "batch-size", 0, "Max files per batch (default: 15)")
}

func init() {
	registerMigrateFlags(rootCmd.Flags())

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a Laravel project to Go + Flutter (same as running `veloce` with no args)",
		RunE:  runMigrate,
	}
	registerMigrateFlags(migrateCmd.Flags())
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(migrateFlags)
	if err != nil {
		if errors.Is(err, config.ErrNotLaravel) {
			PrintHelpfulHint()
		} else if errors.Is(err, config.ErrMissingAPIKey) {
			PrintMissingAPIKeyHint()
		}
		return err
	}

	// Scan.
	files, err := scanner.Scan(cfg.Source)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	// Header.
	PrintRunHeader(cfg.Source, cfg.Output, len(files), phaseBreakdown(files), cfg.BudgetLimit, cfg.DryRun)

	if cfg.DryRun {
		for _, f := range files {
			fmt.Printf("  %s  %s  %s\n",
				paint(cBlue, fmt.Sprintf("phase %d", f.Phase)),
				paint(cPurple, fmt.Sprintf("%-10s", f.Kind)),
				paint(cWhite, f.RelPath),
			)
		}
		return nil
	}

	// State.
	mig, err := state.LoadMigrationState(cfg.Output)
	if err != nil {
		mig = state.NewMigrationState(cfg.Output)
	}
	mig.SetTotalFiles(len(files))
	for _, f := range files {
		if _, ok := mig.Get(f.RelPath); !ok {
			mig.Mark(f.RelPath, state.FileEntry{Status: state.StatusPending, Phase: f.Phase})
		}
	}

	tu, err := state.LoadTokenUsage(cfg.Output, cfg.BudgetLimit)
	if err != nil {
		tu = state.NewTokenUsage(cfg.Output, cfg.BudgetLimit)
	}

	// API clients (OpenRouter).
	// Build worker chain: primary + fallbacks tried in order on 429.
	limiter := openrouter.NewRateLimiter(cfg.RPM, time.Duration(cfg.Delay)*time.Second)
	architect := openrouter.NewArchitectClient(cfg.APIKey)
	openrouter.AttachLimiter(architect, limiter)
	openrouter.AttachAppMetadata(architect, "https://github.com/Nestorservice/veloce", "Veloce")

	allModels := append([]string{openrouter.DefaultWorkerModel}, openrouter.FallbackWorkerModels...)
	workerClients := make([]openrouter.Client, len(allModels))
	for i, model := range allModels {
		c := openrouter.NewWorkerClientWithModel(cfg.APIKey, model)
		openrouter.AttachLimiter(c, limiter)
		openrouter.AttachAppMetadata(c, "https://github.com/Nestorservice/veloce", "Veloce")
		workerClients[i] = c
	}
	currentWorkerIdx := 0

	fmt.Printf("  %s %s\n", paint(cGray, "Worker  :"), paint(cCyan, workerClients[0].Model()))
	fmt.Printf("  %s %s\n", paint(cGray, "Fallback:"), paint(cDim+cGray, strings.Join(allModels[1:], " → ")))
	fmt.Printf("  %s %s\n", paint(cGray, "Planner :"), paint(cPurple, architect.Model()))
	fmt.Printf("  %s %s rpm  delay=%ds  batch=%d files  workers=%d\n",
		paint(cGray, "Rate    :"), paint(cWhite, fmt.Sprintf("%d", cfg.RPM)),
		cfg.Delay, cfg.BatchSize, cfg.Workers)
	fmt.Println()

	bw := &agent.BatchWorker{
		OutputRoot:  cfg.Output,
		Worker:      workerClients[currentWorkerIdx],
		Architect:   architect,
		Migration:   mig,
		TokenUsage:  tu,
		SystemRules: embeddedRules,
		MaxRetries:  3,
	}

	// DeepSeek V4 Flash (free) has a 1M-token context window, so input budget
	// is generous. We cap at 100K input to keep responses focused and avoid
	// timeouts. User can override with --batch-size.
	const freeTierInputCap = 100_000 // safe for 15-20 typical PHP files
	batchOpts := batcher.Options{
		MaxFiles:       cfg.BatchSize,
		MaxInputTokens: freeTierInputCap,
	}

	// Group into batches per phase (phases run strictly in order).
	batches, err := batcher.Group(cfg.Source, files, batchOpts)
	if err != nil {
		return fmt.Errorf("batch grouping: %w", err)
	}

	// Sort batches by phase.
	sort.Slice(batches, func(i, j int) bool { return batches[i].Phase < batches[j].Phase })

	ctx := context.Background()
	dailyQuotaHit := false
	currentPhase := 0
	totalBatches := len(batches)
	doneBatches := 0

	for _, b := range batches {
		if dailyQuotaHit {
			break
		}

		// Phase transition banner.
		if b.Phase != currentPhase {
			if currentPhase != 0 {
				fmt.Println(paint(cGreen+cBold, fmt.Sprintf("✓ Phase %d complete", currentPhase)))
			}
			currentPhase = b.Phase
			phaseBatches := countBatchesInPhase(batches, currentPhase)
			fmt.Println()
			fmt.Println(paint(cBlue+cBold,
				fmt.Sprintf("▶ Phase %d  —  %d batch(es) / %d file(s)",
					currentPhase, phaseBatches, countFilesInPhase(files, currentPhase))))
		}

		// Skip already-done batches.
		if allDone(mig, b) {
			doneBatches++
			fmt.Printf("  %s  %s  %s\n",
				paint(cGreen, "✓ cached "),
				paint(cDim+cGray, fmt.Sprintf("[%d/%d]", doneBatches, totalBatches)),
				paint(cDim+cGray, fmt.Sprintf("%s (%d files)", b.ID, len(b.Files))),
			)
			continue
		}

		doneBatches++
		fmt.Printf("  %s  %s  %s\n",
			paint(cYellow, "→ batch  "),
			paint(cDim+cGray, fmt.Sprintf("[%d/%d]", doneBatches, totalBatches)),
			paint(cWhite, fmt.Sprintf("%s  (%d files, ~%d tokens)", b.ID, len(b.Files), b.InputTokens)),
		)

		// Show the PHP files going into this batch and their expected output paths.
		for i, f := range b.Files {
			connector := "├─"
			if i == len(b.Files)-1 {
				connector = "└─"
			}
			var dst string
			if f.Phase == 4 {
				dst = openrouter.DartOutputPath(f.RelPath)
			} else {
				dst = openrouter.GoOutputPath(f.RelPath)
			}
			fmt.Printf("  %s %s  %s  %s\n",
				paint(cDim+cGray, connector),
				paint(cWhite, f.RelPath),
				paint(cDim+cGray, "→"),
				paint(cDim+cCyan, dst),
			)
		}

		// Process with automatic fallback on 429 rate-limit errors.
		type fileResult struct {
			src, out string
			ok       bool
		}
		var (
			lastErr     error
			dur         time.Duration
			fileResults []fileResult
		)

		for {
			// Reset per-attempt state.
			fileResults = nil
			bw.OnFileWritten = func(src, out string, ok bool) {
				fileResults = append(fileResults, fileResult{src, out, ok})
			}

			// Live spinner.
			progressCh, stopSpinner := startSpinner()
			progressFn := func(msg string) {
				select {
				case progressCh <- msg:
				default:
				}
			}
			openrouter.AttachProgress(bw.Worker, progressFn)
			openrouter.AttachProgress(architect, progressFn)

			t0 := time.Now()
			lastErr = bw.Process(ctx, b)
			stopSpinner()
			dur = time.Since(t0).Round(100 * time.Millisecond)

			_ = mig.Save()
			_ = tu.Save()

			if lastErr == nil {
				break // success
			}
			if errors.Is(lastErr, agent.ErrDailyQuota) {
				break // fatal — handled below
			}

			// 429 rate-limit → try next model in fallback chain.
			if isRateLimitErr(lastErr) && currentWorkerIdx < len(workerClients)-1 {
				currentWorkerIdx++
				bw.Worker = workerClients[currentWorkerIdx]
				fmt.Printf("  %s  %s  %s\n",
					paint(cYellow+cBold, "⚡ fallback"),
					paint(cDim+cGray, "rate-limited · switching to"),
					paint(cCyan, bw.Worker.Model()),
				)
				resetBatchToPending(mig, b)
				_ = mig.Save()
				continue // retry same batch with new model
			}
			break // non-429 or no more fallbacks
		}

		if errors.Is(lastErr, agent.ErrDailyQuota) {
			dailyQuotaHit = true
			fmt.Printf("  %s  %s\n",
				paint(cPink+cBold, "✗ quota  "),
				paint(cPink, "daily free-tier quota exhausted"),
			)
			break
		}
		if lastErr != nil {
			fmt.Printf("  %s  %s  %s\n",
				paint(cPink, "✗ fail   "),
				paint(cDim+cGray, dur.String()),
				paint(cPink, lastErr.Error()),
			)
			continue
		}

		// Success — show confirmed per-file results.
		written := countDone(mig, b)
		fmt.Printf("  %s  %s  %s\n",
			paint(cGreen, "✓ done   "),
			paint(cDim+cGray, dur.String()),
			paint(cWhite, fmt.Sprintf("%d/%d files written", written, len(b.Files))),
		)
		for _, r := range fileResults {
			if r.ok {
				fmt.Printf("       %s %s  →  %s\n",
					paint(cGreen, "✓"),
					paint(cWhite, r.src),
					paint(cCyan, r.out),
				)
			} else {
				fmt.Printf("       %s %s  →  %s\n",
					paint(cPink, "✗"),
					paint(cWhite, r.src),
					paint(cPink, "(aucun output généré)"),
				)
			}
		}
	}

	if currentPhase != 0 && !dailyQuotaHit {
		fmt.Println(paint(cGreen+cBold, fmt.Sprintf("✓ Phase %d complete", currentPhase)))
	}

	fin, fout, pin, pout := tu.Snapshot()
	fmt.Println()
	log.Printf("Done. Tokens worker=%d/%d  architect=%d/%d  total=%d",
		fin, fout, pin, pout, tu.Total())

	if dailyQuotaHit {
		printDailyQuotaHint(cfg)
	}
	return nil
}

// ---- helpers ---------------------------------------------------------------

func allDone(mig *state.MigrationState, b batcher.Batch) bool {
	for _, f := range b.Files {
		e, ok := mig.Get(f.RelPath)
		if !ok || e.Status != state.StatusDone {
			return false
		}
	}
	return true
}

func countDone(mig *state.MigrationState, b batcher.Batch) int {
	n := 0
	for _, f := range b.Files {
		if e, ok := mig.Get(f.RelPath); ok && e.Status == state.StatusDone {
			n++
		}
	}
	return n
}

func countBatchesInPhase(batches []batcher.Batch, phase int) int {
	n := 0
	for _, b := range batches {
		if b.Phase == phase {
			n++
		}
	}
	return n
}

func countFilesInPhase(files []scanner.File, phase int) int {
	n := 0
	for _, f := range files {
		if f.Phase == phase {
			n++
		}
	}
	return n
}

// isRateLimitErr returns true when the error means "this model is unavailable
// right now — try a different one". This covers:
//   - 429 rate-limit (provider temporarily overloaded)
//   - 404 "No endpoints found" (all provider instances for this model are offline)
func isRateLimitErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "429") ||
		(strings.Contains(s, "404") && strings.Contains(s, "No endpoints found"))
}

// resetBatchToPending resets every file in the batch back to StatusPending so
// the batch can be retried with a different model.
func resetBatchToPending(mig *state.MigrationState, b batcher.Batch) {
	for _, f := range b.Files {
		mig.Mark(f.RelPath, state.FileEntry{
			Status: state.StatusPending,
			Phase:  f.Phase,
		})
	}
}

// startSpinner starts a background goroutine that prints a live-updating
// progress line (spinner + elapsed time + latest status message).
// The returned channel accepts status strings from the HTTP client;
// call stop() to erase the spinner line and terminate the goroutine.
func startSpinner() (progressCh chan string, stop func()) {
	ch := make(chan string, 32)
	quit := make(chan struct{})

	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		frame := 0
		t0 := time.Now()
		msg := "connecting…"
		for {
			select {
			case <-quit:
				fmt.Print("\r\033[K") // erase spinner line
				return
			case m := <-ch:
				msg = m
			case <-tick.C:
				elapsed := time.Since(t0).Round(time.Second)
				fmt.Printf("\r  %s  %s  %s\033[K",
					paint(cYellow, frames[frame%len(frames)]),
					paint(cDim+cGray, elapsed.String()),
					paint(cDim+cGray, " · "+msg),
				)
				frame++
			}
		}
	}()

	return ch, func() { close(quit) }
}

func printDailyQuotaHint(_ *config.Config) {
	fmt.Println()
	fmt.Println(paint(cYellow+cBold, "⚠  Daily free-tier quota exhausted (OpenRouter ~200 requests/day)."))
	fmt.Println(paint(cWhite, "   Your progress is saved — re-run `veloce` tomorrow to continue."))
	fmt.Println()
	fmt.Println(paint(cWhite+cBold, "   Or top up OpenRouter ($5 gets ~10 000 requests):"))
	fmt.Println(paint(cGreen, "   https://openrouter.ai/credits"))
	fmt.Println(paint(cDim+cGray, "   Then re-run with: veloce --rpm 200 --workers 10 --delay 1"))
}
