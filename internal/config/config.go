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
	Provider  string // "openrouter" | "groq" | "" (auto-detect)
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
	Provider  string // "openrouter" | "groq"
	// Legacy — kept for token tracker compatibility.
	BudgetLimit int
}

// Load validates flags, applies defaults, and resolves env vars.
// Provider auto-detection: if GROQ_API_KEY is set → groq, else → openrouter.
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
		Provider:    f.Provider,
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

	// --- API key + provider auto-detection ---------------------------------
	// Priority for Groq:        flag --api-key > GROQ_API_KEY env > .env GROQ_API_KEY
	// Priority for OpenRouter:  flag --api-key > OPENROUTER_API_KEY env > .env > GEMINI_API_KEY

	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		groqKey = readDotEnv(c.Source, "GROQ_API_KEY")
	}
	orKey := os.Getenv("OPENROUTER_API_KEY")
	if orKey == "" {
		orKey = readDotEnv(c.Source, "OPENROUTER_API_KEY")
	}
	if orKey == "" {
		orKey = os.Getenv("GEMINI_API_KEY") // backward compat
	}

	// Auto-detect provider if not forced via flag.
	if c.Provider == "" {
		if groqKey != "" {
			c.Provider = "groq"
		} else {
			c.Provider = "openrouter"
		}
	}

	// Resolve the API key for the chosen provider.
	if c.APIKey == "" {
		if c.Provider == "groq" {
			c.APIKey = groqKey
		} else {
			c.APIKey = orKey
		}
	}

	if c.APIKey == "" && !c.DryRun {
		return nil, ErrMissingAPIKey
	}

	// --- defaults (tuned per provider) ------------------------------------
	if c.Workers == 0 {
		c.Workers = 3
	}
	if c.RPM == 0 {
		if c.Provider == "groq" {
			c.RPM = 30 // Groq free tier is much more generous (~30 rpm)
		} else {
			c.RPM = 10 // OpenRouter free tier ~10 rpm
		}
	}
	if c.Workers > c.RPM {
		c.Workers = c.RPM
	}
	if c.Delay == 0 {
		if c.Provider == "groq" {
			c.Delay = 3 // Groq is fast and stable — short pause is enough
		} else {
			c.Delay = 10 // OpenRouter free tier needs longer pause
		}
	}
	if c.BatchSize == 0 {
		c.BatchSize = 5
	}
	if c.BudgetLimit == 0 {
		c.BudgetLimit = 5_000_000
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
	ErrMissingAPIKey = errors.New("API key required: set GROQ_API_KEY or OPENROUTER_API_KEY (or pass --api-key)")
)
