//go:build e2e

package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nestorservice/veloce/internal/agent"
	"github.com/Nestorservice/veloce/internal/gemini"
	"github.com/Nestorservice/veloce/internal/pipeline"
	"github.com/Nestorservice/veloce/internal/scanner"
	"github.com/Nestorservice/veloce/internal/state"
)

type stub struct{ resp string }

func (s *stub) Name() string { return "stub" }
func (s *stub) Complete(ctx context.Context, _ gemini.CompletionRequest) (*gemini.CompletionResponse, error) {
	return &gemini.CompletionResponse{Text: s.resp, InputTokens: 5, OutputTokens: 3}, nil
}

func TestE2E_MinimalProjectProducesGoFiles(t *testing.T) {
	out := t.TempDir()
	os.MkdirAll(filepath.Join(out, "backend"), 0o755)
	os.WriteFile(filepath.Join(out, "backend/go.mod"), []byte("module e2e\n\ngo 1.22\n"), 0o644)

	files, _ := scanner.Scan("../../testdata/laravel_min")
	if len(files) == 0 {
		t.Fatal("no files scanned")
	}

	flash := &stub{resp: "package domain\ntype User struct{ ID string; Email string }\n"}
	pro := &stub{resp: "package domain\ntype User struct{ ID string; Email string }\n"}
	mig := state.NewMigrationState(out)
	st := state.NewSharedTypes(out)
	tu := state.NewTokenUsage(out, 1_000_000)
	corrector := pipeline.NewCorrector(flash, pro, func(string) pipeline.VerifyResult { return pipeline.VerifyResult{OK: true} })

	w := &agent.Worker{
		SourceRoot:  "../../testdata/laravel_min",
		OutputRoot:  out,
		Flash:       flash,
		Pro:         pro,
		Corrector:   corrector,
		Migration:   mig,
		SharedTypes: st,
		TokenUsage:  tu,
		SystemRules: "",
	}

	for _, f := range files {
		if f.Kind != "model" {
			continue
		}
		if err := w.Process(context.Background(), f); err != nil {
			t.Errorf("Process %s: %v", f.RelPath, err)
		}
	}
	e, _ := mig.Get("app/Models/User.php")
	if e.Status != state.StatusDone {
		t.Errorf("status=%s, lastErr=%s", e.Status, e.LastError)
	}
	if _, err := os.Stat(filepath.Join(out, e.Output)); err != nil {
		t.Errorf("output file missing: %v", err)
	}
}
