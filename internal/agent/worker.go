package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nestor/veloce/internal/gemini"
	"github.com/nestor/veloce/internal/output"
	"github.com/nestor/veloce/internal/pipeline"
	"github.com/nestor/veloce/internal/scanner"
	"github.com/nestor/veloce/internal/state"
)

type Worker struct {
	SourceRoot  string
	OutputRoot  string
	Flash       gemini.Client
	Pro         gemini.Client
	Corrector   *pipeline.Corrector
	Migration   *state.MigrationState
	SharedTypes *state.SharedTypes
	TokenUsage  *state.TokenUsage
	SystemRules string
	CachedID    string
}

func (w *Worker) Process(ctx context.Context, f scanner.File) error {
	target := "go"
	if f.Kind == "blade" {
		target = "dart"
	}

	src, err := os.ReadFile(f.AbsPath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	w.Migration.Mark(f.RelPath, state.FileEntry{Status: state.StatusProcessing, Phase: f.Phase, Attempts: 0})

	// Initial generation
	prompt := gemini.BuildTranslationPrompt(gemini.TranslationRequest{
		Target:      target,
		PhaseKind:   f.Kind,
		SourcePath:  f.RelPath,
		SourceCode:  string(src),
		SharedTypes: w.SharedTypes.RenderForPrompt(),
		ArchHint:    archHint(f.Kind),
	})
	resp, err := w.Flash.Complete(ctx, gemini.CompletionRequest{
		SystemRules: w.SystemRules,
		CachedID:    w.CachedID,
		Prompt:      prompt,
	})
	if err != nil {
		w.fail(f.RelPath, f.Phase, 1, err.Error())
		return err
	}
	w.TokenUsage.AddFlash(resp.InputTokens, resp.OutputTokens)
	w.TokenUsage.AddCachedSaved(resp.CachedTokens)

	initialCode := cleanCode(resp.Text)

	if _, err := writeByTarget(w.OutputRoot, f.RelPath, target, initialCode); err != nil {
		w.fail(f.RelPath, f.Phase, 1, err.Error())
		return err
	}

	// Per-run verifier writes the candidate then runs the toolchain.
	verify := func(code string) pipeline.VerifyResult {
		if _, err := writeByTarget(w.OutputRoot, f.RelPath, target, code); err != nil {
			return pipeline.VerifyResult{OK: false, Stderr: err.Error()}
		}
		if target == "go" {
			return pipeline.VerifyGo(filepath.Join(w.OutputRoot, "backend"))
		}
		return pipeline.VerifyDart(filepath.Join(w.OutputRoot, "frontend"))
	}
	w.Corrector.SetVerifyForRun(verify)

	result, err := w.Corrector.Correct(ctx, pipeline.CorrectInput{Target: target, InitialCode: initialCode})
	if err != nil {
		w.fail(f.RelPath, f.Phase, result.Attempts, err.Error())
		return err
	}
	w.TokenUsage.AddFlash(result.FlashIn, result.FlashOut)
	w.TokenUsage.AddPro(result.ProIn, result.ProOut)

	relOut, _ := writeByTarget(w.OutputRoot, f.RelPath, target, result.FinalCode)

	if !result.Success {
		w.fail(f.RelPath, f.Phase, result.Attempts, result.LastError)
		return nil
	}

	if target == "go" {
		for _, t := range pipeline.ExtractGoTypes(result.FinalCode) {
			t.File = relOut
			w.SharedTypes.AddGoType(t)
		}
	} else {
		for _, t := range pipeline.ExtractDartTypes(result.FinalCode) {
			t.File = relOut
			w.SharedTypes.AddDartType(t)
		}
	}

	w.Migration.Mark(f.RelPath, state.FileEntry{
		Status:   state.StatusDone,
		Phase:    f.Phase,
		Output:   relOut,
		Attempts: result.Attempts,
	})
	return nil
}

func (w *Worker) fail(rel string, phase, attempts int, err string) {
	w.Migration.Mark(rel, state.FileEntry{
		Status:    state.StatusFailed,
		Phase:     phase,
		Attempts:  attempts,
		LastError: err,
	})
}

func writeByTarget(outRoot, rel, target, content string) (string, error) {
	if target == "go" {
		return output.WriteGoFile(outRoot, rel, content)
	}
	return output.WriteDartFile(outRoot, rel, content)
}

func archHint(kind string) string {
	switch kind {
	case "model":
		return "Domain struct in package domain. No business logic."
	case "controller":
		return "HTTP handler in package handler with chi router signature."
	case "service":
		return "Business logic in package service. No HTTP, no SQL."
	case "request":
		return "Request struct with validation tags in package handler."
	case "config":
		return "Configuration loader in package config using env vars."
	case "route":
		return "Chi router setup in cmd/api/routes.go."
	case "migration":
		return "Raw SQL migration. Output SQL only."
	case "blade":
		return "Flutter StatelessWidget/StatefulWidget in features/<feature>/presentation/screens."
	}
	return ""
}

func cleanCode(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return s
}
