package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/nestor/veloce/internal/agent"
	"github.com/nestor/veloce/internal/config"
	"github.com/nestor/veloce/internal/gemini"
	"github.com/nestor/veloce/internal/pipeline"
	"github.com/nestor/veloce/internal/scanner"
	"github.com/nestor/veloce/internal/state"
)

var migrateFlags config.Flags

func init() {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a Laravel project to Go + Flutter",
		RunE:  runMigrate,
	}
	migrateCmd.Flags().StringVar(&migrateFlags.Source, "source", "", "Path to Laravel project (required)")
	migrateCmd.Flags().StringVar(&migrateFlags.Output, "output", "./output", "Output monorepo path")
	migrateCmd.Flags().IntVar(&migrateFlags.Workers, "workers", 5, "Worker pool size per phase")
	migrateCmd.Flags().IntVar(&migrateFlags.BudgetLimit, "budget", 5_000_000, "Token budget (kill switch)")
	migrateCmd.Flags().StringVar(&migrateFlags.APIKey, "api-key", "", "Gemini API key (or $GEMINI_API_KEY)")
	migrateCmd.Flags().BoolVar(&migrateFlags.Resume, "resume", false, "Resume from last checkpoint")
	migrateCmd.Flags().BoolVar(&migrateFlags.DryRun, "dry-run", false, "Skip API + writes")
	migrateCmd.Flags().BoolVar(&migrateFlags.RunTests, "run-tests", false, "Run go test / flutter test after each file")

	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(migrateFlags)
	if err != nil {
		return err
	}

	files, err := scanner.Scan(cfg.Source)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	log.Printf("scanner: %d files classified", len(files))

	var mig *state.MigrationState
	if cfg.Resume {
		if mig, err = state.LoadMigrationState(cfg.Output); err != nil {
			return fmt.Errorf("resume: %w", err)
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

	rules, err := os.ReadFile("configs/default_rules.md")
	if err != nil {
		return fmt.Errorf("read default_rules: %w", err)
	}

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
