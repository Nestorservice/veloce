package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Flags is the raw set of CLI flag values.
type Flags struct {
	Source    string
	Output    string
	Workers   int
	APIKey    string
	Resume    bool
	DryRun    bool
	RunTests  bool
	RPM       int
	Delay     int // forced pause in seconds between batches
	BatchSize int // max files per batch
	// Legacy Gemini budget field — kept so existing callers compile.
	BudgetLimit int
}

// Config is the validated, defaulted runtime configuration.
type Config struct {
	Source    string
	Output    string
	Workers   int
	APIKey    string
	Resume    bool
	DryRun    bool
	RunTests  bool
	RPM       int
	Delay     int
	BatchSize int
	// Legacy — kept for token tracker compatibility.
	BudgetLimit int
}

// Load validates flags, applies defaults, and resolves env vars.
// It also reads $SOURCE/.env if present so OPENROUTER_API_KEY can live there.
func Load(f Flags) (*Config, error) {
	c := &Config{
		Source:      f.Source,
		Output:      f.Output,
		Workers:     f.Workers,
		APIKey:      f.APIKey,
		Resume:      f.Resume,
		DryRun:      f.DryRun,
		RunTests:    f.RunTests,
		RPM:         f.RPM,
		Delay:       f.Delay,
		BatchSize:   f.BatchSize,
		BudgetLimit: f.BudgetLimit,
	}

	// --- source resolution -------------------------------------------------
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
		return nil, fmt.Errorf("%w: %s (missing composer.json/artisan/app/)", ErrNotLaravel, c.Source)
	}

	// --- output path -------------------------------------------------------
	if c.Output == "" {
		parent := filepath.Dir(c.Source)
		name := filepath.Base(c.Source)
		c.Output = filepath.Join(parent, name+"_output")
	}

	// --- defaults ----------------------------------------------------------
	if c.Workers == 0 {
		c.Workers = 3 // conservative for free-tier OpenRouter
	}
	if c.RPM == 0 {
		c.RPM = 10 // OpenRouter free tier allows ~10 rpm
	}
	if c.Workers > c.RPM {
		c.Workers = c.RPM
	}
	if c.Delay == 0 {
		c.Delay = 10 // 10s forced pause between batches (free tier safety)
	}
	if c.BatchSize == 0 {
		// Free-tier DeepSeek context = 65 536 tokens. 5 files × ~2 000 tok/file
		// input leaves ~55 000 tokens for output — comfortable margin.
		c.BatchSize = 5
	}
	if c.BudgetLimit == 0 {
		c.BudgetLimit = 5_000_000
	}

	// --- API key -----------------------------------------------------------
	// Priority: flag > OPENROUTER_API_KEY env > .env file > legacy GEMINI_API_KEY env
	if c.APIKey == "" {
		c.APIKey = os.Getenv("OPENROUTER_API_KEY")
	}
	if c.APIKey == "" {
		c.APIKey = readDotEnv(c.Source, "OPENROUTER_API_KEY")
	}
	if c.APIKey == "" {
		// Fallback: allow old Gemini key env var for backward compat.
		c.APIKey = os.Getenv("GEMINI_API_KEY")
	}
	if c.APIKey == "" && !c.DryRun {
		return nil, ErrMissingAPIKey
	}

	return c, nil
}

// looksLikeLaravel returns true if dir contains enough Laravel markers.
func looksLikeLaravel(dir string) bool {
	hits := 0
	for _, marker := range []string{"composer.json", "artisan", "app", "routes", "config"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			hits++
		}
	}
	return hits >= 2
}

// readDotEnv reads a KEY=value pair from <projectRoot>/.env (if it exists).
// Returns "" if the file or the key cannot be found.
func readDotEnv(root, key string) string {
	f, err := os.Open(filepath.Join(root, ".env"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	prefix := key + "="
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, prefix) {
			continue
		}
		val := strings.TrimPrefix(line, prefix)
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}

var (
	ErrNotLaravel    = errors.New("not a Laravel project")
	ErrMissingAPIKey = errors.New("API key required: pass --api-key or set OPENROUTER_API_KEY")
)
