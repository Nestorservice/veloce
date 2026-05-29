package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	RPM         int
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
	RPM         int
}

// Load validates flags, applies defaults, resolves env vars.
func Load(f Flags) (*Config, error) {
	c := &Config{
		Source:      f.Source,
		Output:      f.Output,
		Workers:     f.Workers,
		BudgetLimit: f.BudgetLimit,
		APIKey:      f.APIKey,
		Resume:      f.Resume,
		DryRun:      f.DryRun,
		RunTests:    f.RunTests,
		RPM:         f.RPM,
	}
	if c.Source == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
		c.Source = cwd
	}
	abs, err := filepath.Abs(c.Source)
	if err != nil {
		return nil, fmt.Errorf("resolve source: %w", err)
	}
	c.Source = abs
	if !looksLikeLaravel(c.Source) {
		return nil, fmt.Errorf("not a Laravel project: %s (missing composer.json/artisan/app/)", c.Source)
	}
	if c.Output == "" {
		parent := filepath.Dir(c.Source)
		name := filepath.Base(c.Source)
		c.Output = filepath.Join(parent, name+"_output")
	}
	if c.Workers == 0 {
		c.Workers = 5
	}
	if c.RPM == 0 {
		c.RPM = 5 // safe default = Gemini free-tier limit
	}
	// no point running more workers than the rate limit allows
	if c.Workers > c.RPM {
		c.Workers = c.RPM
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

func looksLikeLaravel(dir string) bool {
	hits := 0
	for _, marker := range []string{"composer.json", "artisan", "app", "routes", "config"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			hits++
		}
	}
	return hits >= 2
}

var ErrNotLaravel = errors.New("not a Laravel project")
