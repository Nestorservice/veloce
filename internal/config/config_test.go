package config

import (
	"os"
	"testing"
)

func TestLoad_DefaultsApplied(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "key-from-env")
	defer os.Unsetenv("GEMINI_API_KEY")

	c, err := Load(Flags{Source: "./src", Output: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Output != "./output" {
		t.Errorf("Output default = %q, want ./output", c.Output)
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

func TestLoad_MissingSourceErrors(t *testing.T) {
	if _, err := Load(Flags{}); err == nil {
		t.Fatal("expected error for missing --source")
	}
}

func TestLoad_MissingAPIKeyErrors(t *testing.T) {
	os.Unsetenv("GEMINI_API_KEY")
	if _, err := Load(Flags{Source: "./src"}); err == nil {
		t.Fatal("expected error for missing API key")
	}
}
