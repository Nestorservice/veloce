package config

import (
	"errors"
	"os"
)

// Flags is the raw set of CLI flag values.
type Flags struct {
	Source      string
	Output      string
	Workers     int
	BudgetLimit int
	APIKey      string
	Resume      bool
	DryRun      bool
	RunTests    bool
}

// Config is the validated, defaulted runtime configuration.
type Config struct {
	Source      string
	Output      string
	Workers     int
	BudgetLimit int
	APIKey      string
	Resume      bool
	DryRun      bool
	RunTests    bool
}

// Load validates flags, applies defaults, resolves env vars.
func Load(f Flags) (*Config, error) {
	if f.Source == "" {
		return nil, errors.New("--source is required")
	}

	c := &Config{
		Source:      f.Source,
		Output:      f.Output,
		Workers:     f.Workers,
		BudgetLimit: f.BudgetLimit,
		APIKey:      f.APIKey,
		Resume:      f.Resume,
		DryRun:      f.DryRun,
		RunTests:    f.RunTests,
	}
	if c.Output == "" {
		c.Output = "./output"
	}
	if c.Workers == 0 {
		c.Workers = 5
	}
	if c.BudgetLimit == 0 {
		c.BudgetLimit = 5_000_000
	}
	if c.APIKey == "" {
		c.APIKey = os.Getenv("GEMINI_API_KEY")
	}
	if c.APIKey == "" && !c.DryRun {
		return nil, errors.New("API key required: pass --api-key or set GEMINI_API_KEY")
	}
	return c, nil
}
