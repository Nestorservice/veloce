package config

import (
	"os"
	"path/filepath"
	"testing"
)

func laravelDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestLoad_DefaultsApplied(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "key-from-env")
	defer os.Unsetenv("GEMINI_API_KEY")

	src := laravelDir(t)
	c, err := Load(Flags{Source: src, Output: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOut := filepath.Join(filepath.Dir(src), filepath.Base(src)+"_output")
	if c.Output != wantOut {
		t.Errorf("Output default = %q, want %q", c.Output, wantOut)
	}
	if c.Workers != 5 {
		t.Errorf("Workers default = %d, want 5", c.Workers)
	}
	if c.BudgetLimit != 5_000_000 {
		t.Errorf("BudgetLimit default = %d, want 5_000_000", c.BudgetLimit)
	}
	if c.APIKey != "key-from-env" {
		t.Errorf("APIKey = %q, want from env", c.APIKey)
	}
}

func TestLoad_NotLaravelErrors(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "k")
	defer os.Unsetenv("GEMINI_API_KEY")
	if _, err := Load(Flags{Source: t.TempDir()}); err == nil {
		t.Fatal("expected error for non-Laravel dir")
	}
}

func TestLoad_MissingAPIKeyErrors(t *testing.T) {
	os.Unsetenv("GEMINI_API_KEY")
	if _, err := Load(Flags{Source: laravelDir(t)}); err == nil {
		t.Fatal("expected error for missing API key")
	}
}
