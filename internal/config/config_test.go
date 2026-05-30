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
	os.Setenv("OPENROUTER_API_KEY", "key-from-env")
	defer os.Unsetenv("OPENROUTER_API_KEY")

	src := laravelDir(t)
	c, err := Load(Flags{Source: src, Output: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOut := filepath.Join(filepath.Dir(src), filepath.Base(src)+"_output")
	if c.Output != wantOut {
		t.Errorf("Output = %q, want %q", c.Output, wantOut)
	}
	if c.Workers != 3 {
		t.Errorf("Workers = %d, want 3 (OpenRouter free default)", c.Workers)
	}
	if c.RPM != 10 {
		t.Errorf("RPM = %d, want 10", c.RPM)
	}
	if c.BatchSize != 15 {
		t.Errorf("BatchSize = %d, want 15", c.BatchSize)
	}
	if c.Delay != 10 {
		t.Errorf("Delay = %d, want 10", c.Delay)
	}
	if c.BudgetLimit != 5_000_000 {
		t.Errorf("BudgetLimit = %d, want 5_000_000", c.BudgetLimit)
	}
	if c.APIKey != "key-from-env" {
		t.Errorf("APIKey = %q, want from env", c.APIKey)
	}
}

func TestLoad_DotEnvAPIKey(t *testing.T) {
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	src := laravelDir(t)
	dotEnv := "OPENROUTER_API_KEY=sk-from-dotenv\nDB_HOST=localhost\n"
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte(dotEnv), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(Flags{Source: src})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.APIKey != "sk-from-dotenv" {
		t.Errorf("APIKey = %q, want sk-from-dotenv", c.APIKey)
	}
}

func TestLoad_NotLaravelErrors(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "k")
	defer os.Unsetenv("OPENROUTER_API_KEY")
	if _, err := Load(Flags{Source: t.TempDir()}); err == nil {
		t.Fatal("expected error for non-Laravel dir")
	}
}

func TestLoad_MissingAPIKeyErrors(t *testing.T) {
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	if _, err := Load(Flags{Source: laravelDir(t)}); err == nil {
		t.Fatal("expected error for missing API key")
	}
}
