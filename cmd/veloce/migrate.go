package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Nestorservice/veloce/internal/agent"
	"github.com/Nestorservice/veloce/internal/config"
	"github.com/Nestorservice/veloce/internal/gemini"
	"github.com/Nestorservice/veloce/internal/pipeline"
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
	flags.IntVar(&migrateFlags.Workers, "workers", 5, "Worker pool size per phase")
	flags.IntVar(&migrateFlags.BudgetLimit, "budget", 5_000_000, "Token budget (kill switch)")
	flags.StringVar(&migrateFlags.APIKey, "api-key", "", "Gemini API key (or $GEMINI_API_KEY)")
	flags.BoolVar(&migrateFlags.Resume, "resume", true, "Resume from last checkpoint if present")
	flags.BoolVar(&migrateFlags.DryRun, "dry-run", false, "Skip API + writes (analysis only)")
	flags.BoolVar(&migrateFlags.RunTests, "run-tests", false, "Run go test / flutter test after each file")
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
		PrintHelpfulHint()
		return err
	}

	files, err := scanner.Scan(cfg.Source)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	PrintRunHeader(cfg.Source, cfg.Output, len(files), phaseBreakdown(files), cfg.BudgetLimit, cfg.DryRun)

	if cfg.DryRun {
		for _, f := range files {
			fmt.Printf("  %s  %s  %s\n",
				paint(cBlue, fmt.Sprintf("phase %d", f.Phase)),
				paint(cPurple, fmt.Sprintf("%-7s", f.Kind)),
				paint(cWhite, f.RelPath),
			)
		}
		return nil
	}

	var mig *state.MigrationState
	if cfg.Resume {
		mig, err = state.LoadMigrationState(cfg.Output)
		if err != nil {
			// no prior run yet — start fresh
			mig = state.NewMigrationState(cfg.Output)
		}
	} else {
		mig = state.NewMigrationState(cfg.Output)
	}
	mig.SetTotalFiles(len(files))
	for _, f := range files {
		if _, ok := mig.Get(f.RelPath); !ok {
			mig.Mark(f.RelPath, state.FileEntry{Status: state.StatusPending, Phase: f.Phase})
		}
	}

	st, _ := state.LoadSharedTypes(cfg.Output)
	tu, _ := state.LoadTokenUsage(cfg.Output, cfg.BudgetLimit)

	flash := gemini.NewFlashClient(cfg.APIKey)
	pro := gemini.NewProClient(cfg.APIKey)

	rules := []byte(embeddedRules)

	cm := gemini.NewCacheManager(cfg.APIKey, cfg.Output)
	cacheID, _ := cm.LoadCacheID()
	if cacheID == "" && !cfg.DryRun {
		cacheID, err = cm.EnsureCache(string(rules), st.RenderForPrompt())
		if err != nil {
			log.Printf("cache create failed (continuing without cache): %v", err)
			cacheID = ""
		}
	}

	corrector := pipeline.NewCorrector(flash, pro, func(string) pipeline.VerifyResult { return pipeline.VerifyResult{OK: true} })

	worker := &agent.Worker{
		SourceRoot:  cfg.Source,
		OutputRoot:  cfg.Output,
		Flash:       flash,
		Pro:         pro,
		Corrector:   corrector,
		Migration:   mig,
		SharedTypes: st,
		TokenUsage:  tu,
		SystemRules: string(rules),
		CachedID:    cacheID,
	}

	processFn := func(ctx context.Context, f scanner.File) error {
		if tu.Exceeded() {
			return fmt.Errorf("budget exhausted")
		}
		if entry, ok := mig.Get(f.RelPath); ok && entry.Status == state.StatusDone {
			return nil
		}
		err := worker.Process(ctx, f)
		_ = mig.Save()
		_ = st.Save()
		_ = tu.Save()
		return err
	}

	orch := &agent.Orchestrator{
		Files:       files,
		Workers:     cfg.Workers,
		ProcessFn:   processFn,
		BudgetCheck: tu.Exceeded,
		OnPhaseEnd: func(p int) {
			_ = mig.Save()
			_ = st.Save()
			_ = tu.Save()
			log.Printf("phase %d complete", p)
		},
	}

	ctx := context.Background()
	if err := orch.Run(ctx); err != nil {
		log.Printf("orchestrator: %v", err)
	}

	_ = mig.Save()
	_ = st.Save()
	_ = tu.Save()

	fin, fout, pin, pout := tu.Snapshot()
	log.Printf("Done. Tokens flash=%d/%d pro=%d/%d total=%d",
		fin, fout, pin, pout, tu.Total())
	return nil
}
