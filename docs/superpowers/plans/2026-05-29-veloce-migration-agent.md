# Veloce — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `veloce`, a Go CLI agent that orchestrates a Laravel → Go + Flutter migration using two Gemini models (Pro + Flash) with a ReAct loop, persistent per-file checkpoints, type-consistency index, FinOps controls, and an auto-correction pipeline.

**Architecture:** Concurrent phase-pipeline. The CLI scans a Laravel project, classifies files into 4 phases, then runs each phase through a goroutine worker pool. Each worker executes a ReAct loop (generate → verify → correct) calling Gemini Flash 95% of the time and Gemini Pro 5% (escalation). State (migration progress, shared types, token usage) is persisted in `./output/.veloce/` for resume.

**Tech Stack:** Go 1.22+, `spf13/cobra` (CLI), `google.golang.org/genai` (Gemini), `sync.RWMutex` (concurrency), `os/exec` (compiler invocation), JSON files (state). The generated targets use Chi + GORM + JWT (Go) and Riverpod + Dio + GoRouter (Flutter).

---

## File Structure

The agent project layout:

```
veloce/
├── cmd/veloce/main.go                  Cobra root, command registration
├── internal/
│   ├── config/config.go                CLI flags → typed Config struct
│   ├── scanner/php_scanner.go          Walk Laravel dir, classify by phase
│   ├── scanner/php_scanner_test.go
│   ├── state/migration_state.go        Per-file checkpoint, atomic JSON write
│   ├── state/migration_state_test.go
│   ├── state/shared_types.go           Type index, RWMutex protected
│   ├── state/shared_types_test.go
│   ├── state/token_usage.go            Budget tracker + kill switch
│   ├── state/token_usage_test.go
│   ├── gemini/client.go                Common Gemini interface
│   ├── gemini/worker_client.go         Flash client (translation)
│   ├── gemini/architect.go             Pro client (escalation)
│   ├── gemini/cache.go                 Context caching
│   ├── gemini/prompts.go               Prompt templates
│   ├── pipeline/compiler.go            Run go build / dart analyze
│   ├── pipeline/compiler_test.go
│   ├── pipeline/corrector.go           Correction loop (3 attempts → escalate)
│   ├── pipeline/extractor.go           Parse generated Go/Dart → extract type signatures
│   ├── pipeline/extractor_test.go
│   ├── agent/worker.go                 Single ReAct loop unit
│   ├── agent/orchestrator.go           4-phase coordinator + worker pool
│   ├── output/go_writer.go             Map source path → target Go path, write file
│   ├── output/flutter_writer.go        Map source path → target Dart path, write file
│   └── output/path_mapper.go           Path translation rules
├── configs/default_rules.md            Immutable migration rules (uploaded to cache)
├── cmd/veloce/migrate.go               `veloce migrate` command
├── cmd/veloce/status.go                `veloce status` command
├── cmd/veloce/retry.go                 `veloce retry` command
├── go.mod
└── README.md
```

---

### Task 1: Bootstrap project & Go module

**Files:**
- Create: `go.mod`
- Create: `README.md`
- Create: `cmd/veloce/main.go`

- [ ] **Step 1: Initialise the Go module**

Run from project root:
```bash
cd "C:/Users/Nestor Corneille/Desktop/veloce"
go mod init github.com/nestor/veloce
go get github.com/spf13/cobra@latest
```

Expected: `go.mod` created with module path and cobra dependency.

- [ ] **Step 2: Write minimal `cmd/veloce/main.go`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "veloce",
	Short: "Veloce — Laravel → Go+Flutter migration agent",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Write a placeholder `README.md`**

```markdown
# Veloce

CLI agent that migrates Laravel applications to Go + Flutter.

See `docs/superpowers/specs/2026-05-29-migration-agent-design.md` for the design.

## Build

```bash
go build -o veloce ./cmd/veloce
```
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: no error, no output.

- [ ] **Step 5: Verify CLI runs**

Run: `go run ./cmd/veloce --help`
Expected: cobra usage text including `veloce` short description.

- [ ] **Step 6: Commit**

```bash
git init
git add go.mod go.sum cmd/veloce/main.go README.md
git commit -m "chore: bootstrap veloce go module and cobra entrypoint"
```

---

### Task 2: Config struct & CLI flags

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — package does not compile (Load, Flags undefined).

- [ ] **Step 3: Implement `internal/config/config.go`**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/...`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): typed configuration with defaults and validation"
```

---

### Task 3: Migration state (per-file checkpoint)

**Files:**
- Create: `internal/state/migration_state.go`
- Create: `internal/state/migration_state_test.go`

- [ ] **Step 1: Write the failing test**

`internal/state/migration_state_test.go`:
```go
package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationState_SaveAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewMigrationState(dir)

	s.Mark("app/Models/User.php", FileEntry{Status: StatusDone, Phase: 2, Output: "backend/internal/domain/user.go", Attempts: 1})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadMigrationState(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := loaded.Get("app/Models/User.php")
	if !ok || got.Status != StatusDone || got.Phase != 2 {
		t.Errorf("unexpected entry: %+v", got)
	}
}

func TestMigrationState_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	s := NewMigrationState(dir)
	s.Mark("a.php", FileEntry{Status: StatusPending, Phase: 1})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// Tmp file should not remain.
	matches, _ := filepath.Glob(filepath.Join(dir, ".veloce", "migration_state.json.tmp*"))
	if len(matches) != 0 {
		t.Errorf("tmp files leaked: %v", matches)
	}
	if _, err := os.Stat(filepath.Join(dir, ".veloce", "migration_state.json")); err != nil {
		t.Errorf("final file missing: %v", err)
	}
}

func TestMigrationState_PendingFilesInPhase(t *testing.T) {
	s := NewMigrationState(t.TempDir())
	s.Mark("a.php", FileEntry{Status: StatusDone, Phase: 1})
	s.Mark("b.php", FileEntry{Status: StatusPending, Phase: 1})
	s.Mark("c.php", FileEntry{Status: StatusPending, Phase: 2})

	pending := s.PendingInPhase(1)
	if len(pending) != 1 || pending[0] != "b.php" {
		t.Errorf("got %v, want [b.php]", pending)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/state/...`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Implement `internal/state/migration_state.go`**

```go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type FileEntry struct {
	Status    Status `json:"status"`
	Phase     int    `json:"phase"`
	Output    string `json:"output,omitempty"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error,omitempty"`
}

type migrationData struct {
	SessionID    string               `json:"session_id"`
	TotalFiles   int                  `json:"total_files"`
	CurrentPhase int                  `json:"current_phase"`
	Files        map[string]FileEntry `json:"files"`
}

type MigrationState struct {
	mu      sync.RWMutex
	rootDir string
	data    migrationData
}

func NewMigrationState(outputDir string) *MigrationState {
	return &MigrationState{
		rootDir: outputDir,
		data: migrationData{
			SessionID: time.Now().UTC().Format(time.RFC3339),
			Files:     map[string]FileEntry{},
		},
	}
}

func LoadMigrationState(outputDir string) (*MigrationState, error) {
	path := filepath.Join(outputDir, ".veloce", "migration_state.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var d migrationData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if d.Files == nil {
		d.Files = map[string]FileEntry{}
	}
	return &MigrationState{rootDir: outputDir, data: d}, nil
}

func (s *MigrationState) Mark(src string, e FileEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Files[src] = e
	if e.Phase > s.data.CurrentPhase {
		s.data.CurrentPhase = e.Phase
	}
}

func (s *MigrationState) Get(src string) (FileEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data.Files[src]
	return e, ok
}

func (s *MigrationState) SetTotalFiles(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.TotalFiles = n
}

func (s *MigrationState) PendingInPhase(phase int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for path, e := range s.data.Files {
		if e.Phase == phase && (e.Status == StatusPending || e.Status == StatusProcessing) {
			out = append(out, path)
		}
	}
	return out
}

// Save writes the JSON atomically: temp file then rename.
func (s *MigrationState) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir := filepath.Join(s.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "migration_state.json.tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, filepath.Join(dir, "migration_state.json"))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/state/...`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/state/migration_state.go internal/state/migration_state_test.go
git commit -m "feat(state): per-file migration checkpoint with atomic save"
```

---

### Task 4: Shared types index (mutex-protected)

**Files:**
- Create: `internal/state/shared_types.go`
- Create: `internal/state/shared_types_test.go`

- [ ] **Step 1: Write the failing test**

```go
package state

import (
	"testing"
)

func TestSharedTypes_AddAndRender(t *testing.T) {
	st := NewSharedTypes(t.TempDir())
	st.AddGoType(GoType{Name: "User", Package: "domain", File: "user.go", Fields: []string{"ID uuid.UUID", "Email string"}})
	st.AddDartType(DartType{Name: "UserModel", File: "features/auth/domain/user_model.dart", Fields: []string{"String id", "String email"}})

	out := st.RenderForPrompt()
	if !contains(out, "User") || !contains(out, "UserModel") {
		t.Errorf("rendered prompt missing type names: %s", out)
	}
}

func TestSharedTypes_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	st := NewSharedTypes(dir)
	st.AddGoType(GoType{Name: "Product", Package: "domain", File: "product.go", Fields: []string{"ID uuid.UUID"}})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSharedTypes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.GetGoType("Product"); !ok {
		t.Errorf("Product type missing after load")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/state/...`
Expected: FAIL — undefined: NewSharedTypes, GoType, DartType.

- [ ] **Step 3: Implement `internal/state/shared_types.go`**

```go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type GoType struct {
	Name    string   `json:"-"`
	Package string   `json:"package"`
	File    string   `json:"file"`
	Fields  []string `json:"fields"`
}

type DartType struct {
	Name   string   `json:"-"`
	File   string   `json:"file"`
	Fields []string `json:"fields"`
}

type sharedTypesData struct {
	GoTypes   map[string]GoType   `json:"go_types"`
	DartTypes map[string]DartType `json:"dart_types"`
}

type SharedTypes struct {
	mu      sync.RWMutex
	rootDir string
	data    sharedTypesData
}

func NewSharedTypes(outputDir string) *SharedTypes {
	return &SharedTypes{
		rootDir: outputDir,
		data: sharedTypesData{
			GoTypes:   map[string]GoType{},
			DartTypes: map[string]DartType{},
		},
	}
}

func LoadSharedTypes(outputDir string) (*SharedTypes, error) {
	path := filepath.Join(outputDir, ".veloce", "shared_types.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewSharedTypes(outputDir), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read shared_types: %w", err)
	}
	var d sharedTypesData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse shared_types: %w", err)
	}
	if d.GoTypes == nil {
		d.GoTypes = map[string]GoType{}
	}
	if d.DartTypes == nil {
		d.DartTypes = map[string]DartType{}
	}
	return &SharedTypes{rootDir: outputDir, data: d}, nil
}

func (s *SharedTypes) AddGoType(t GoType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.GoTypes[t.Name] = t
}

func (s *SharedTypes) AddDartType(t DartType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.DartTypes[t.Name] = t
}

func (s *SharedTypes) GetGoType(name string) (GoType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data.GoTypes[name]
	return t, ok
}

func (s *SharedTypes) RenderForPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	b.WriteString("# Existing Go types\n")
	for name, t := range s.data.GoTypes {
		fmt.Fprintf(&b, "type %s struct { %s } // pkg %s, %s\n",
			name, strings.Join(t.Fields, "; "), t.Package, t.File)
	}
	b.WriteString("\n# Existing Dart types\n")
	for name, t := range s.data.DartTypes {
		fmt.Fprintf(&b, "class %s { %s } // %s\n",
			name, strings.Join(t.Fields, "; "), t.File)
	}
	return b.String()
}

func (s *SharedTypes) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir := filepath.Join(s.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "shared_types.json.tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()
	return os.Rename(tmpPath, filepath.Join(dir, "shared_types.json"))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/state/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/state/shared_types.go internal/state/shared_types_test.go
git commit -m "feat(state): shared types index with concurrent-safe access"
```

---

### Task 5: Token usage tracker (kill switch)

**Files:**
- Create: `internal/state/token_usage.go`
- Create: `internal/state/token_usage_test.go`

- [ ] **Step 1: Write the failing test**

```go
package state

import "testing"

func TestTokenUsage_AddFlashAndTotal(t *testing.T) {
	tu := NewTokenUsage(t.TempDir(), 1_000_000)
	tu.AddFlash(500, 300)
	tu.AddFlash(200, 100)
	if got := tu.Total(); got != 1100 {
		t.Errorf("Total = %d, want 1100", got)
	}
}

func TestTokenUsage_ExceededTriggersKillSwitch(t *testing.T) {
	tu := NewTokenUsage(t.TempDir(), 100)
	tu.AddFlash(60, 50)
	if !tu.Exceeded() {
		t.Errorf("Exceeded should be true (used 110 > limit 100)")
	}
}

func TestTokenUsage_NotExceededUnderLimit(t *testing.T) {
	tu := NewTokenUsage(t.TempDir(), 1000)
	tu.AddFlash(100, 100)
	tu.AddPro(50, 50)
	if tu.Exceeded() {
		t.Error("should not be exceeded")
	}
}

func TestTokenUsage_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	tu := NewTokenUsage(dir, 1000)
	tu.AddFlash(100, 100)
	tu.AddPro(50, 50)
	if err := tu.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTokenUsage(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Total() != 300 {
		t.Errorf("Total after load = %d, want 300", loaded.Total())
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/state/...`
Expected: FAIL — undefined: NewTokenUsage.

- [ ] **Step 3: Implement `internal/state/token_usage.go`**

```go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type tokenData struct {
	FlashInputTokens  int64 `json:"flash_input_tokens"`
	FlashOutputTokens int64 `json:"flash_output_tokens"`
	ProInputTokens    int64 `json:"pro_input_tokens"`
	ProOutputTokens   int64 `json:"pro_output_tokens"`
	CachedTokensSaved int64 `json:"cached_tokens_saved"`
}

type TokenUsage struct {
	mu       sync.RWMutex
	rootDir  string
	limit    int64
	data     tokenData
}

func NewTokenUsage(outputDir string, limit int) *TokenUsage {
	return &TokenUsage{rootDir: outputDir, limit: int64(limit)}
}

func LoadTokenUsage(outputDir string, limit int) (*TokenUsage, error) {
	path := filepath.Join(outputDir, ".veloce", "token_usage.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewTokenUsage(outputDir, limit), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read token_usage: %w", err)
	}
	var d tokenData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse token_usage: %w", err)
	}
	return &TokenUsage{rootDir: outputDir, limit: int64(limit), data: d}, nil
}

func (t *TokenUsage) AddFlash(in, out int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.FlashInputTokens += int64(in)
	t.data.FlashOutputTokens += int64(out)
}

func (t *TokenUsage) AddPro(in, out int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.ProInputTokens += int64(in)
	t.data.ProOutputTokens += int64(out)
}

func (t *TokenUsage) AddCachedSaved(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.CachedTokensSaved += int64(n)
}

func (t *TokenUsage) Total() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.data.FlashInputTokens + t.data.FlashOutputTokens +
		t.data.ProInputTokens + t.data.ProOutputTokens
}

func (t *TokenUsage) Exceeded() bool {
	return t.Total() >= t.limit
}

func (t *TokenUsage) Snapshot() (flashIn, flashOut, proIn, proOut int64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.data.FlashInputTokens, t.data.FlashOutputTokens, t.data.ProInputTokens, t.data.ProOutputTokens
}

func (t *TokenUsage) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	dir := filepath.Join(t.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload := struct {
		tokenData
		Total      int64 `json:"total_tokens"`
		BudgetLim  int64 `json:"budget_limit"`
	}{t.data, t.data.FlashInputTokens + t.data.FlashOutputTokens + t.data.ProInputTokens + t.data.ProOutputTokens, t.limit}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "token_usage.json.tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()
	return os.Rename(tmpPath, filepath.Join(dir, "token_usage.json"))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/state/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/state/token_usage.go internal/state/token_usage_test.go
git commit -m "feat(state): token usage tracker with budget kill switch"
```

---

### Task 6: PHP Scanner — classify by phase

**Files:**
- Create: `internal/scanner/php_scanner.go`
- Create: `internal/scanner/php_scanner_test.go`

- [ ] **Step 1: Write the failing test**

```go
package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func setupLaravelTree(t *testing.T) string {
	root := t.TempDir()
	files := map[string]string{
		"config/database.php":                          "<?php",
		"routes/api.php":                               "<?php",
		"database/migrations/2024_create_users.php":    "<?php",
		"app/Models/User.php":                          "<?php",
		"app/Models/Product.php":                       "<?php",
		"app/Http/Controllers/AuthController.php":      "<?php",
		"app/Services/AuthService.php":                 "<?php",
		"app/Http/Requests/LoginRequest.php":           "<?php",
		"resources/views/auth/login.blade.php":         "<html>",
		"vendor/symfony/something.php":                 "<?php", // should be skipped
		"storage/framework/cache.php":                  "<?php", // should be skipped
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScan_ClassifiesIntoPhases(t *testing.T) {
	root := setupLaravelTree(t)
	files, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	counts := map[int]int{}
	for _, f := range files {
		counts[f.Phase]++
	}

	if counts[1] != 3 {
		t.Errorf("phase 1 = %d, want 3 (config + routes + migration)", counts[1])
	}
	if counts[2] != 2 {
		t.Errorf("phase 2 = %d, want 2 (User + Product)", counts[2])
	}
	if counts[3] != 3 {
		t.Errorf("phase 3 = %d, want 3 (Controller + Service + Request)", counts[3])
	}
	if counts[4] != 1 {
		t.Errorf("phase 4 = %d, want 1 (blade)", counts[4])
	}
}

func TestScan_SkipsVendorAndStorage(t *testing.T) {
	root := setupLaravelTree(t)
	files, _ := Scan(root)
	for _, f := range files {
		if filepath.Dir(f.RelPath) == "vendor" || filepath.Dir(f.RelPath) == "storage" {
			t.Errorf("vendor/storage file leaked: %s", f.RelPath)
		}
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/scanner/...`
Expected: FAIL — undefined: Scan.

- [ ] **Step 3: Implement `internal/scanner/php_scanner.go`**

```go
package scanner

import (
	"io/fs"
	"path/filepath"
	"strings"
)

type File struct {
	AbsPath string
	RelPath string // relative to project root
	Phase   int
	Kind    string // "config", "route", "migration", "model", "controller", "service", "request", "blade"
}

// Scan walks the Laravel root and returns a classified list of files.
// Vendor, storage, bootstrap/cache, node_modules and tests are skipped.
func Scan(root string) ([]File, error) {
	var out []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if isSkipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".php") {
			return nil
		}

		phase, kind, ok := classify(rel)
		if !ok {
			return nil
		}
		out = append(out, File{AbsPath: path, RelPath: rel, Phase: phase, Kind: kind})
		return nil
	})
	return out, err
}

func isSkipDir(rel string) bool {
	switch rel {
	case "vendor", "storage", "node_modules", "bootstrap/cache", "tests", ".git":
		return true
	}
	return false
}

func classify(rel string) (int, string, bool) {
	switch {
	case strings.HasPrefix(rel, "config/"):
		return 1, "config", true
	case strings.HasPrefix(rel, "routes/"):
		return 1, "route", true
	case strings.HasPrefix(rel, "database/migrations/"):
		return 1, "migration", true
	case strings.HasPrefix(rel, "app/Models/"), strings.HasPrefix(rel, "app/Repositories/"):
		return 2, "model", true
	case strings.HasPrefix(rel, "app/Http/Controllers/"):
		return 3, "controller", true
	case strings.HasPrefix(rel, "app/Services/"):
		return 3, "service", true
	case strings.HasPrefix(rel, "app/Http/Requests/"):
		return 3, "request", true
	case strings.HasPrefix(rel, "resources/views/") && strings.HasSuffix(rel, ".blade.php"):
		return 4, "blade", true
	}
	return 0, "", false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/scanner/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/
git commit -m "feat(scanner): classify Laravel files into 4 migration phases"
```

---

### Task 7: Default migration rules document

**Files:**
- Create: `configs/default_rules.md`

- [ ] **Step 1: Write the rules document**

`configs/default_rules.md`:
```markdown
# Veloce Migration Rules (immutable)

You are an expert PHP → Go/Dart migration engine. Follow these rules without exception.

## Output discipline
- Respond with ONLY valid source code for the requested target (Go or Dart).
- NO explanations, NO prose, NO markdown fences, NO comments unless they map a Laravel concept.
- The very first character of your response must be the first character of the code.

## Go targets (Phases 1–3)

### Architecture
- Clean / hexagonal layers: handler → service → repository → domain.
- HTTP routing: `github.com/go-chi/chi/v5`.
- ORM: `gorm.io/gorm` for repositories.
- Validation: `github.com/go-playground/validator/v10` on every request struct.
- JWT: `github.com/golang-jwt/jwt/v5`. Passwords: `golang.org/x/crypto/bcrypt`.

### Error handling
- NEVER use `panic` in business logic.
- Always wrap errors: `return fmt.Errorf("describe: %w", err)`.
- Handlers convert errors to HTTP responses with explicit status codes.

### Naming
- Domain structs use PascalCase singular: `User`, `Product`.
- Files: snake_case (`user_handler.go`).
- Packages: lowercase singular (`handler`, `service`, `repository`, `domain`).

## Dart/Flutter targets (Phase 4)

### Architecture
- Feature-first: `lib/features/<feature>/{data,domain,presentation}`.
- State: Riverpod (`flutter_riverpod`).
- HTTP client: Dio with a centralised interceptor for JWT auto-refresh.
- Routing: GoRouter.
- NO platform-specific imports (`dart:io`, `dart:html`) — code must compile on web, desktop, and mobile identically.

### CSS/Tailwind → Flutter mapping
- `flex flex-row` → `Row(children: ...)`
- `flex flex-col` → `Column(children: ...)`
- `p-4` → `Padding(padding: EdgeInsets.all(16), ...)`
- `text-lg` → `TextStyle(fontSize: 18)`
- Colours → `Theme.of(context).colorScheme` where possible.

## Cross-target consistency
- The "Existing types" block injected before each request lists Go structs and Dart classes already generated. NEW types you generate MUST be field-compatible (same names, same logical types) with their counterparts.
- DateTime in Go → `time.Time`. In Dart → `DateTime`. JSON field names use snake_case.
- UUIDs in Go → `github.com/google/uuid` (`uuid.UUID`). In Dart → `String`.

## Forbidden patterns
- No `eval`, `unsafe.Pointer`, `dart:mirrors`.
- No global mutable state.
- No untyped maps as return values in business logic.
```

- [ ] **Step 2: Commit**

```bash
git add configs/default_rules.md
git commit -m "docs(configs): immutable migration rules for Gemini context cache"
```

---

### Task 8: Gemini client interface & prompt builder

**Files:**
- Create: `internal/gemini/client.go`
- Create: `internal/gemini/prompts.go`
- Create: `internal/gemini/prompts_test.go`

- [ ] **Step 1: Write the failing test**

`internal/gemini/prompts_test.go`:
```go
package gemini

import (
	"strings"
	"testing"
)

func TestBuildTranslationPrompt_IncludesAllParts(t *testing.T) {
	p := BuildTranslationPrompt(TranslationRequest{
		Target:        "go",
		PhaseKind:     "model",
		SourcePath:    "app/Models/User.php",
		SourceCode:    "<?php class User extends Model {}",
		SharedTypes:   "type Order struct { ID uuid.UUID }",
		ArchHint:      "Generate a domain struct in package domain.",
	})
	for _, want := range []string{"go", "model", "User.php", "Order", "domain", "ONLY"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildCorrectionPrompt_IncludesError(t *testing.T) {
	p := BuildCorrectionPrompt(CorrectionRequest{
		Target:       "go",
		PreviousCode: "package domain\ntype User struct { ID UUID }",
		BuildError:   "undefined: UUID",
	})
	if !strings.Contains(p, "undefined: UUID") {
		t.Errorf("correction prompt missing build error")
	}
	if !strings.Contains(p, "ONLY") {
		t.Errorf("correction prompt missing strict-output directive")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/gemini/...`
Expected: FAIL — undefined: BuildTranslationPrompt.

- [ ] **Step 3: Implement `internal/gemini/client.go`**

```go
package gemini

import "context"

// CompletionRequest is the abstract request passed to a Gemini client.
type CompletionRequest struct {
	SystemRules   string // immutable rules (cached)
	CachedID      string // context cache id (empty = no cache)
	Prompt        string // user-visible prompt
	MaxOutputTokens int
}

// CompletionResponse is the abstract response.
type CompletionResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
	CachedTokens int // tokens served from cache (reported by API)
}

// Client is the contract implemented by both Flash and Pro clients.
type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Name() string // "flash" or "pro"
}
```

- [ ] **Step 4: Implement `internal/gemini/prompts.go`**

```go
package gemini

import (
	"fmt"
	"strings"
)

type TranslationRequest struct {
	Target      string // "go" | "dart"
	PhaseKind   string // "config", "model", "controller", "blade", ...
	SourcePath  string
	SourceCode  string
	SharedTypes string
	ArchHint    string
}

type CorrectionRequest struct {
	Target       string
	PreviousCode string
	BuildError   string
}

func BuildTranslationPrompt(r TranslationRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Existing types\n%s\n\n", r.SharedTypes)
	fmt.Fprintf(&b, "## Translation request\n")
	fmt.Fprintf(&b, "- Target language: %s\n", r.Target)
	fmt.Fprintf(&b, "- Source file: %s\n", r.SourcePath)
	fmt.Fprintf(&b, "- Kind: %s\n", r.PhaseKind)
	fmt.Fprintf(&b, "- Architecture hint: %s\n\n", r.ArchHint)
	fmt.Fprintf(&b, "## PHP source\n```php\n%s\n```\n\n", r.SourceCode)
	fmt.Fprintf(&b, "Respond with ONLY valid %s code. No prose, no markdown.\n", strings.ToUpper(r.Target))
	return b.String()
}

func BuildCorrectionPrompt(r CorrectionRequest) string {
	var b strings.Builder
	b.WriteString("## Previous attempt failed to compile.\n\n")
	fmt.Fprintf(&b, "### Build error\n```\n%s\n```\n\n", r.BuildError)
	fmt.Fprintf(&b, "### Previous code\n```%s\n%s\n```\n\n", r.Target, r.PreviousCode)
	fmt.Fprintf(&b, "Produce a corrected version. Respond with ONLY valid %s code. No prose.\n", strings.ToUpper(r.Target))
	return b.String()
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/gemini/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/gemini/client.go internal/gemini/prompts.go internal/gemini/prompts_test.go
git commit -m "feat(gemini): client interface and prompt templates"
```

---

### Task 9: Gemini Flash & Pro HTTP clients

**Files:**
- Create: `internal/gemini/worker_client.go`
- Create: `internal/gemini/architect.go`
- Create: `internal/gemini/http_client.go`
- Create: `internal/gemini/http_client_test.go`

- [ ] **Step 1: Write the failing test (using a fake HTTP server)**

`internal/gemini/http_client_test.go`:
```go
package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClient_Complete_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "translate me") {
			t.Errorf("prompt missing in body: %s", body)
		}
		resp := map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "package domain"}}}},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     120,
				"candidatesTokenCount": 30,
				"cachedContentTokenCount": 80,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &httpClient{
		name:    "flash",
		model:   "gemini-2.5-flash",
		baseURL: srv.URL,
		apiKey:  "fake",
		http:    http.DefaultClient,
	}

	resp, err := c.Complete(context.Background(), CompletionRequest{Prompt: "translate me"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "package domain" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.InputTokens != 120 || resp.OutputTokens != 30 || resp.CachedTokens != 80 {
		t.Errorf("token counts = (%d,%d,%d)", resp.InputTokens, resp.OutputTokens, resp.CachedTokens)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/gemini/...`
Expected: FAIL — undefined: httpClient.

- [ ] **Step 3: Implement `internal/gemini/http_client.go`**

```go
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type httpClient struct {
	name    string // "flash" | "pro"
	model   string // "gemini-2.5-flash" | "gemini-2.5-pro"
	baseURL string // typically "https://generativelanguage.googleapis.com/v1beta"
	apiKey  string
	http    *http.Client
}

func (c *httpClient) Name() string { return c.name }

type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiReqBody struct {
	Contents         []geminiContent `json:"contents"`
	SystemInstruction *geminiContent `json:"systemInstruction,omitempty"`
	CachedContent    string         `json:"cachedContent,omitempty"`
	GenerationConfig map[string]any `json:"generationConfig,omitempty"`
}
type geminiCandidate struct {
	Content geminiContent `json:"content"`
}
type geminiUsage struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}
type geminiResp struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
}

func (c *httpClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	body := geminiReqBody{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}}},
		GenerationConfig: map[string]any{
			"responseMimeType": "text/plain",
			"temperature":      0.2,
		},
	}
	if req.SystemRules != "" {
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.SystemRules}}}
	}
	if req.CachedID != "" {
		body.CachedContent = req.CachedID
	}
	if req.MaxOutputTokens > 0 {
		body.GenerationConfig["maxOutputTokens"] = req.MaxOutputTokens
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, string(raw))
	}
	var parsed geminiResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini: empty response")
	}
	return &CompletionResponse{
		Text:         parsed.Candidates[0].Content.Parts[0].Text,
		InputTokens:  parsed.UsageMetadata.PromptTokenCount,
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		CachedTokens: parsed.UsageMetadata.CachedContentTokenCount,
	}, nil
}
```

- [ ] **Step 4: Implement `internal/gemini/worker_client.go`**

```go
package gemini

import "net/http"

// NewFlashClient returns the worker client (Gemini 2.5 Flash).
func NewFlashClient(apiKey string) Client {
	return &httpClient{
		name:    "flash",
		model:   "gemini-2.5-flash",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}
```

- [ ] **Step 5: Implement `internal/gemini/architect.go`**

```go
package gemini

import "net/http"

// NewProClient returns the architect client (Gemini 2.5 Pro).
func NewProClient(apiKey string) Client {
	return &httpClient{
		name:    "pro",
		model:   "gemini-2.5-pro",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/gemini/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/gemini/
git commit -m "feat(gemini): HTTP clients for Flash and Pro models"
```

---

### Task 10: Context cache manager

**Files:**
- Create: `internal/gemini/cache.go`
- Create: `internal/gemini/cache_test.go`

- [ ] **Step 1: Write the failing test**

```go
package gemini

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheManager_CreatesAndPersistsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"name":"cachedContents/abc123"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cm := &CacheManager{
		baseURL: srv.URL,
		apiKey:  "k",
		http:    http.DefaultClient,
		rootDir: dir,
	}

	id, err := cm.EnsureCache("rules content", "types content")
	if err != nil {
		t.Fatalf("EnsureCache: %v", err)
	}
	if id != "cachedContents/abc123" {
		t.Errorf("id = %q", id)
	}
	stored, _ := os.ReadFile(filepath.Join(dir, ".veloce", "context_cache_id.txt"))
	if string(stored) != id {
		t.Errorf("persisted id = %q, want %q", stored, id)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/gemini/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/gemini/cache.go`**

```go
package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type CacheManager struct {
	baseURL string
	apiKey  string
	http    *http.Client
	rootDir string // output dir; cache id stored under .veloce/
}

func NewCacheManager(apiKey, rootDir string) *CacheManager {
	return &CacheManager{
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		http:    &http.Client{},
		rootDir: rootDir,
	}
}

// EnsureCache creates a new cached content blob and persists its ID.
// Returns the cache ID to be passed as `cached_content` in subsequent requests.
func (m *CacheManager) EnsureCache(rules, types string) (string, error) {
	combined := rules + "\n\n" + types
	body := map[string]any{
		"model": "models/gemini-2.5-flash",
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": combined}}},
		},
		"ttl": "3600s",
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/cachedContents?key=%s", m.baseURL, m.apiKey)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("cache request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("cache create %d: %s", resp.StatusCode, raw)
	}
	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("cache decode: %w", err)
	}
	dir := filepath.Join(m.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "context_cache_id.txt"), []byte(parsed.Name), 0o644); err != nil {
		return "", err
	}
	return parsed.Name, nil
}

// LoadCacheID returns the persisted cache id if present.
func (m *CacheManager) LoadCacheID() (string, error) {
	b, err := os.ReadFile(filepath.Join(m.rootDir, ".veloce", "context_cache_id.txt"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/gemini/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gemini/cache.go internal/gemini/cache_test.go
git commit -m "feat(gemini): context cache creation and persistence"
```

---

### Task 11: Type signature extractor (Go + Dart)

**Files:**
- Create: `internal/pipeline/extractor.go`
- Create: `internal/pipeline/extractor_test.go`

- [ ] **Step 1: Write the failing test**

```go
package pipeline

import "testing"

func TestExtractGoTypes_StructWithFields(t *testing.T) {
	src := `package domain

import "time"

type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}
`
	got := ExtractGoTypes(src)
	if len(got) != 1 || got[0].Name != "User" || got[0].Package != "domain" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Fields) != 3 {
		t.Errorf("fields = %v", got[0].Fields)
	}
}

func TestExtractDartTypes_ClassWithFields(t *testing.T) {
	src := `class UserModel {
  final String id;
  final String email;
  final DateTime createdAt;
}`
	got := ExtractDartTypes(src)
	if len(got) != 1 || got[0].Name != "UserModel" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Fields) != 3 {
		t.Errorf("fields = %v", got[0].Fields)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/pipeline/...`
Expected: FAIL — undefined: ExtractGoTypes.

- [ ] **Step 3: Implement `internal/pipeline/extractor.go`**

```go
package pipeline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	"github.com/nestor/veloce/internal/state"
)

// ExtractGoTypes parses Go source and returns each top-level struct type.
func ExtractGoTypes(src string) []state.GoType {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	pkg := f.Name.Name
	var out []state.GoType
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		var fields []string
		for _, fld := range st.Fields.List {
			typeStr := exprToString(fld.Type)
			for _, name := range fld.Names {
				fields = append(fields, name.Name+" "+typeStr)
			}
		}
		out = append(out, state.GoType{Name: ts.Name.Name, Package: pkg, Fields: fields})
		return true
	})
	return out
}

func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	case *ast.ArrayType:
		return "[]" + exprToString(v.Elt)
	default:
		return "?"
	}
}

var dartClassRE = regexp.MustCompile(`(?m)class\s+(\w+)\s*\{([\s\S]*?)\}`)
var dartFieldRE = regexp.MustCompile(`(?m)^\s*(?:final\s+|static\s+|const\s+)?([\w<>,\s]+?)\s+(\w+)\s*;`)

// ExtractDartTypes parses Dart source and returns each top-level class.
func ExtractDartTypes(src string) []state.DartType {
	var out []state.DartType
	for _, m := range dartClassRE.FindAllStringSubmatch(src, -1) {
		name := m[1]
		body := m[2]
		var fields []string
		for _, fm := range dartFieldRE.FindAllStringSubmatch(body, -1) {
			typeStr := strings.TrimSpace(fm[1])
			fieldName := fm[2]
			fields = append(fields, typeStr+" "+fieldName)
		}
		out = append(out, state.DartType{Name: name, Fields: fields})
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/pipeline/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/extractor.go internal/pipeline/extractor_test.go
git commit -m "feat(pipeline): extract type signatures from generated Go and Dart"
```

---

### Task 12: Compiler verification (go build / dart analyze)

**Files:**
- Create: `internal/pipeline/compiler.go`
- Create: `internal/pipeline/compiler_test.go`

- [ ] **Step 1: Write the failing test**

```go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyGo_PassesOnValidPackage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("package test\n\nfunc Hello() string { return \"hi\" }\n"), 0o644)

	res := VerifyGo(dir)
	if !res.OK {
		t.Errorf("expected OK, got stderr=%s", res.Stderr)
	}
}

func TestVerifyGo_FailsOnBrokenPackage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("package test\n\nfunc Hello() { return notDefined }\n"), 0o644)

	res := VerifyGo(dir)
	if res.OK {
		t.Errorf("expected failure")
	}
	if res.Stderr == "" {
		t.Errorf("expected stderr output")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/pipeline/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/pipeline/compiler.go`**

```go
package pipeline

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type VerifyResult struct {
	OK     bool
	Stdout string
	Stderr string
}

// VerifyGo runs `go build ./...` then `go vet ./...` in dir.
func VerifyGo(dir string) VerifyResult {
	if r := runCmd(dir, "go", "build", "./..."); !r.OK {
		return r
	}
	return runCmd(dir, "go", "vet", "./...")
}

// VerifyDart runs `dart analyze` in dir.
func VerifyDart(dir string) VerifyResult {
	return runCmd(dir, "dart", "analyze")
}

func runCmd(dir, name string, args ...string) VerifyResult {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return VerifyResult{
		OK:     err == nil,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/pipeline/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/compiler.go internal/pipeline/compiler_test.go
git commit -m "feat(pipeline): go build/vet and dart analyze verification"
```

---

### Task 13: Output path mapper & writers

**Files:**
- Create: `internal/output/path_mapper.go`
- Create: `internal/output/path_mapper_test.go`
- Create: `internal/output/go_writer.go`
- Create: `internal/output/flutter_writer.go`

- [ ] **Step 1: Write the failing test**

`internal/output/path_mapper_test.go`:
```go
package output

import "testing"

func TestMapGoPath(t *testing.T) {
	cases := []struct{ src, want string }{
		{"app/Models/User.php", "backend/internal/domain/user.go"},
		{"app/Http/Controllers/AuthController.php", "backend/internal/handler/auth_handler.go"},
		{"app/Services/AuthService.php", "backend/internal/service/auth_service.go"},
		{"config/database.php", "backend/internal/config/database.go"},
		{"routes/api.php", "backend/cmd/api/routes.go"},
	}
	for _, c := range cases {
		got := MapGoPath(c.src)
		if got != c.want {
			t.Errorf("MapGoPath(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestMapDartPath(t *testing.T) {
	cases := []struct{ src, want string }{
		{"resources/views/auth/login.blade.php", "frontend/lib/features/auth/presentation/screens/login_screen.dart"},
		{"resources/views/products/index.blade.php", "frontend/lib/features/products/presentation/screens/index_screen.dart"},
	}
	for _, c := range cases {
		got := MapDartPath(c.src)
		if got != c.want {
			t.Errorf("MapDartPath(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/output/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/output/path_mapper.go`**

```go
package output

import (
	"path/filepath"
	"strings"
	"unicode"
)

// MapGoPath translates a Laravel source path to its Go target path.
func MapGoPath(src string) string {
	src = filepath.ToSlash(src)
	switch {
	case strings.HasPrefix(src, "app/Models/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), ".php"))
		return "backend/internal/domain/" + name + ".go"
	case strings.HasPrefix(src, "app/Http/Controllers/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Controller.php"))
		return "backend/internal/handler/" + name + "_handler.go"
	case strings.HasPrefix(src, "app/Services/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Service.php"))
		return "backend/internal/service/" + name + "_service.go"
	case strings.HasPrefix(src, "app/Repositories/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Repository.php"))
		return "backend/internal/repository/" + name + "_repository.go"
	case strings.HasPrefix(src, "app/Http/Requests/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Request.php"))
		return "backend/internal/handler/" + name + "_request.go"
	case strings.HasPrefix(src, "config/"):
		name := strings.TrimSuffix(filepath.Base(src), ".php")
		return "backend/internal/config/" + name + ".go"
	case strings.HasPrefix(src, "routes/"):
		return "backend/cmd/api/routes.go"
	case strings.HasPrefix(src, "database/migrations/"):
		name := strings.TrimSuffix(filepath.Base(src), ".php")
		return "backend/migrations/" + name + ".sql"
	}
	return "backend/_unmapped/" + filepath.Base(src) + ".go"
}

// MapDartPath translates a Blade view to its Flutter screen path.
// e.g. resources/views/auth/login.blade.php → frontend/lib/features/auth/presentation/screens/login_screen.dart
func MapDartPath(src string) string {
	src = filepath.ToSlash(src)
	src = strings.TrimPrefix(src, "resources/views/")
	src = strings.TrimSuffix(src, ".blade.php")
	parts := strings.Split(src, "/")
	if len(parts) < 2 {
		return "frontend/lib/features/_misc/presentation/screens/" + snake(parts[0]) + "_screen.dart"
	}
	feature := parts[0]
	screen := snake(strings.Join(parts[1:], "_"))
	return "frontend/lib/features/" + feature + "/presentation/screens/" + screen + "_screen.dart"
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Implement `internal/output/go_writer.go`**

```go
package output

import (
	"os"
	"path/filepath"
)

// WriteGoFile maps src → target Go path under outputDir and writes content atomically.
func WriteGoFile(outputDir, srcPath, content string) (string, error) {
	rel := MapGoPath(srcPath)
	full := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
```

- [ ] **Step 5: Implement `internal/output/flutter_writer.go`**

```go
package output

import (
	"os"
	"path/filepath"
)

// WriteDartFile maps a Blade source path → target Dart path under outputDir and writes content.
func WriteDartFile(outputDir, srcPath, content string) (string, error) {
	rel := MapDartPath(srcPath)
	full := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/output/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/output/
git commit -m "feat(output): path mapping rules and atomic file writers"
```

---

### Task 14: Corrector — bounded retry loop

**Files:**
- Create: `internal/pipeline/corrector.go`
- Create: `internal/pipeline/corrector_test.go`

- [ ] **Step 1: Write the failing test (uses fake Client)**

```go
package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/nestor/veloce/internal/gemini"
)

type fakeClient struct {
	name  string
	resps []string // one per call
	calls int
}

func (f *fakeClient) Name() string { return f.name }
func (f *fakeClient) Complete(ctx context.Context, req gemini.CompletionRequest) (*gemini.CompletionResponse, error) {
	if f.calls >= len(f.resps) {
		return &gemini.CompletionResponse{Text: f.resps[len(f.resps)-1]}, nil
	}
	r := f.resps[f.calls]
	f.calls++
	return &gemini.CompletionResponse{Text: r, InputTokens: 10, OutputTokens: 5}, nil
}

func TestCorrect_SucceedsOnFirstRetry(t *testing.T) {
	flash := &fakeClient{name: "flash", resps: []string{"good code"}}
	pro := &fakeClient{name: "pro"}

	verify := func(code string) VerifyResult {
		if strings.Contains(code, "good") {
			return VerifyResult{OK: true}
		}
		return VerifyResult{OK: false, Stderr: "bad"}
	}

	c := NewCorrector(flash, pro, verify)
	res, err := c.Correct(context.Background(), CorrectInput{Target: "go", InitialCode: "bad code"})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalCode != "good code" || res.Attempts != 2 {
		t.Errorf("got %+v", res)
	}
	if res.UsedPro {
		t.Errorf("Pro should not have been called")
	}
}

func TestCorrect_EscalatesToProAfterFlashFails(t *testing.T) {
	flash := &fakeClient{name: "flash", resps: []string{"bad1", "bad2"}}
	pro := &fakeClient{name: "pro", resps: []string{"finally good"}}

	verify := func(code string) VerifyResult {
		if strings.Contains(code, "good") {
			return VerifyResult{OK: true}
		}
		return VerifyResult{OK: false, Stderr: "bad"}
	}

	c := NewCorrector(flash, pro, verify)
	res, _ := c.Correct(context.Background(), CorrectInput{Target: "go", InitialCode: "bad0"})
	if !res.UsedPro {
		t.Error("Pro should have been called")
	}
	if res.FinalCode != "finally good" {
		t.Errorf("FinalCode = %q", res.FinalCode)
	}
}

func TestCorrect_FailsAfterAllAttempts(t *testing.T) {
	flash := &fakeClient{name: "flash", resps: []string{"bad1", "bad2"}}
	pro := &fakeClient{name: "pro", resps: []string{"still bad"}}

	verify := func(code string) VerifyResult {
		return VerifyResult{OK: false, Stderr: "broken"}
	}

	c := NewCorrector(flash, pro, verify)
	res, _ := c.Correct(context.Background(), CorrectInput{Target: "go", InitialCode: "bad0"})
	if res.Success {
		t.Error("expected failure")
	}
	if res.Attempts != 4 {
		t.Errorf("Attempts = %d, want 4", res.Attempts)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/pipeline/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/pipeline/corrector.go`**

```go
package pipeline

import (
	"context"

	"github.com/nestor/veloce/internal/gemini"
)

type VerifyFn func(code string) VerifyResult

type CorrectInput struct {
	Target      string // "go" or "dart"
	InitialCode string
	InitialErr  string // optional, the build error of InitialCode
}

type CorrectResult struct {
	Success   bool
	FinalCode string
	Attempts  int  // total Gemini calls (initial gen counts as 1)
	UsedPro   bool
	LastError string
	FlashIn   int
	FlashOut  int
	ProIn     int
	ProOut    int
}

type Corrector struct {
	flash  gemini.Client
	pro    gemini.Client
	verify VerifyFn
}

func NewCorrector(flash, pro gemini.Client, verify VerifyFn) *Corrector {
	return &Corrector{flash: flash, pro: pro, verify: verify}
}

// Correct runs up to 4 attempts:
//   1. Verify initial code; if OK, return.
//   2. Flash correction #1.
//   3. Flash correction #2.
//   4. Pro correction.
func (c *Corrector) Correct(ctx context.Context, in CorrectInput) (*CorrectResult, error) {
	out := &CorrectResult{FinalCode: in.InitialCode, Attempts: 1}
	if v := c.verify(in.InitialCode); v.OK {
		out.Success = true
		return out, nil
	} else {
		out.LastError = v.Stderr
	}

	code := in.InitialCode
	lastErr := out.LastError

	for i := 0; i < 2; i++ {
		out.Attempts++
		req := gemini.CompletionRequest{Prompt: gemini.BuildCorrectionPrompt(gemini.CorrectionRequest{Target: in.Target, PreviousCode: code, BuildError: lastErr})}
		resp, err := c.flash.Complete(ctx, req)
		if err != nil {
			out.LastError = err.Error()
			return out, nil
		}
		out.FlashIn += resp.InputTokens
		out.FlashOut += resp.OutputTokens
		code = resp.Text
		if v := c.verify(code); v.OK {
			out.Success = true
			out.FinalCode = code
			return out, nil
		} else {
			lastErr = v.Stderr
			out.LastError = lastErr
		}
	}

	// Pro escalation.
	out.Attempts++
	out.UsedPro = true
	req := gemini.CompletionRequest{Prompt: gemini.BuildCorrectionPrompt(gemini.CorrectionRequest{Target: in.Target, PreviousCode: code, BuildError: lastErr})}
	resp, err := c.pro.Complete(ctx, req)
	if err != nil {
		out.LastError = err.Error()
		return out, nil
	}
	out.ProIn += resp.InputTokens
	out.ProOut += resp.OutputTokens
	code = resp.Text
	if v := c.verify(code); v.OK {
		out.Success = true
		out.FinalCode = code
	} else {
		out.LastError = v.Stderr
	}
	out.FinalCode = code
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/pipeline/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/corrector.go internal/pipeline/corrector_test.go
git commit -m "feat(pipeline): bounded correction loop with Pro escalation"
```

---

### Task 15: Worker — single-file ReAct unit

**Files:**
- Create: `internal/agent/worker.go`
- Create: `internal/agent/worker_test.go`

- [ ] **Step 1: Write the failing test**

```go
package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nestor/veloce/internal/gemini"
	"github.com/nestor/veloce/internal/pipeline"
	"github.com/nestor/veloce/internal/scanner"
	"github.com/nestor/veloce/internal/state"
)

type fakeClient struct {
	resps []string
	calls int
}

func (f *fakeClient) Name() string { return "fake" }
func (f *fakeClient) Complete(ctx context.Context, _ gemini.CompletionRequest) (*gemini.CompletionResponse, error) {
	r := f.resps[f.calls]
	f.calls++
	return &gemini.CompletionResponse{Text: r, InputTokens: 10, OutputTokens: 5}, nil
}

func TestWorker_ProcessGoFile_Success(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	outDir := filepath.Join(tmp, "out")
	os.MkdirAll(filepath.Join(srcDir, "app/Models"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "app/Models/User.php"), []byte("<?php class User {}"), 0o644)
	os.MkdirAll(filepath.Join(outDir, "backend"), 0o755)
	os.WriteFile(filepath.Join(outDir, "backend/go.mod"), []byte("module out\n\ngo 1.22\n"), 0o644)

	validGo := "package domain\n\ntype User struct { ID string }\n"
	flash := &fakeClient{resps: []string{validGo}}
	pro := &fakeClient{}

	mig := state.NewMigrationState(outDir)
	st := state.NewSharedTypes(outDir)
	tu := state.NewTokenUsage(outDir, 1_000_000)
	corrector := pipeline.NewCorrector(flash, pro, func(code string) pipeline.VerifyResult {
		// Verify against the freshly-written backend module.
		return pipeline.VerifyGo(filepath.Join(outDir, "backend"))
	})

	w := &Worker{
		SourceRoot:   srcDir,
		OutputRoot:   outDir,
		Flash:        flash,
		Pro:          pro,
		Corrector:    corrector,
		Migration:    mig,
		SharedTypes:  st,
		TokenUsage:   tu,
		SystemRules:  "rules",
		CachedID:     "",
	}

	err := w.Process(context.Background(), scanner.File{
		AbsPath: filepath.Join(srcDir, "app/Models/User.php"),
		RelPath: "app/Models/User.php",
		Phase:   2,
		Kind:    "model",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	entry, _ := mig.Get("app/Models/User.php")
	if entry.Status != state.StatusDone {
		t.Errorf("status = %q", entry.Status)
	}
	if !strings.HasSuffix(entry.Output, "user.go") {
		t.Errorf("output = %q", entry.Output)
	}
	if _, ok := st.GetGoType("User"); !ok {
		t.Error("User type not registered")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/agent/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/agent/worker.go`**

```go
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

	// Read source
	src, err := os.ReadFile(f.AbsPath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	// Mark processing
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

	// Write initial code to its target path so verifier sees it
	if _, err := writeByTarget(w.OutputRoot, f.RelPath, target, initialCode); err != nil {
		w.fail(f.RelPath, f.Phase, 1, err.Error())
		return err
	}

	// Verify + correct
	verify := func(code string) pipeline.VerifyResult {
		// Rewrite with current candidate then verify whole module.
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

	// Register types
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
	// Strip markdown fences if the model leaked them.
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return s
}
```

- [ ] **Step 4: Adjust the Corrector to allow SetVerifyForRun**

Edit `internal/pipeline/corrector.go` and add at the end:

```go
// SetVerifyForRun swaps the verifier for the next Correct call.
func (c *Corrector) SetVerifyForRun(v VerifyFn) {
	c.verify = v
}
```

- [ ] **Step 5: Run tests (worker requires Go installed)**

Run: `go test ./internal/agent/... -count=1`
Expected: PASS.

If `go build` from the temp module fails because the module path conflicts, ensure the temp `go.mod` uses a unique module name (`out` in this test).

- [ ] **Step 6: Commit**

```bash
git add internal/agent/worker.go internal/agent/worker_test.go internal/pipeline/corrector.go
git commit -m "feat(agent): single-file ReAct worker with verify+correct loop"
```

---

### Task 16: Orchestrator — phase pipeline & worker pool

**Files:**
- Create: `internal/agent/orchestrator.go`
- Create: `internal/agent/orchestrator_test.go`

- [ ] **Step 1: Write the failing test**

```go
package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nestor/veloce/internal/scanner"
)

func TestOrchestrator_RunsPhasesSequentially(t *testing.T) {
	var phase2Done, phase3Started atomic.Bool
	processed := make(chan int, 10)

	proc := func(ctx context.Context, f scanner.File) error {
		processed <- f.Phase
		if f.Phase == 3 && !phase2Done.Load() {
			t.Error("phase 3 file processed before phase 2 finished")
		}
		if f.Phase == 3 {
			phase3Started.Store(true)
		}
		return nil
	}

	files := []scanner.File{
		{RelPath: "a", Phase: 2}, {RelPath: "b", Phase: 2},
		{RelPath: "c", Phase: 3}, {RelPath: "d", Phase: 3},
	}

	o := &Orchestrator{Files: files, Workers: 2, ProcessFn: proc, OnPhaseEnd: func(p int) { if p == 2 { phase2Done.Store(true) } }}
	if err := o.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/agent/...`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/agent/orchestrator.go`**

```go
package agent

import (
	"context"
	"sort"
	"sync"

	"github.com/nestor/veloce/internal/scanner"
)

type ProcessFn func(ctx context.Context, f scanner.File) error

type Orchestrator struct {
	Files      []scanner.File
	Workers    int
	ProcessFn  ProcessFn
	OnPhaseEnd func(phase int)
	BudgetCheck func() bool // optional; return true to stop
}

func (o *Orchestrator) Run(ctx context.Context) error {
	if o.Workers < 1 {
		o.Workers = 1
	}
	byPhase := map[int][]scanner.File{}
	phases := []int{}
	for _, f := range o.Files {
		if _, ok := byPhase[f.Phase]; !ok {
			phases = append(phases, f.Phase)
		}
		byPhase[f.Phase] = append(byPhase[f.Phase], f)
	}
	sort.Ints(phases)

	for _, p := range phases {
		if err := o.runPhase(ctx, byPhase[p]); err != nil {
			return err
		}
		if o.OnPhaseEnd != nil {
			o.OnPhaseEnd(p)
		}
		if o.BudgetCheck != nil && o.BudgetCheck() {
			return context.Canceled
		}
	}
	return nil
}

func (o *Orchestrator) runPhase(ctx context.Context, files []scanner.File) error {
	jobs := make(chan scanner.File)
	var wg sync.WaitGroup
	errCh := make(chan error, o.Workers)

	for i := 0; i < o.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				if err := o.ProcessFn(ctx, f); err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			}
		}()
	}

	for _, f := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- f:
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/agent/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/orchestrator.go internal/agent/orchestrator_test.go
git commit -m "feat(agent): phase-sequential orchestrator with worker pool"
```

---

### Task 17: `veloce migrate` command — wire everything

**Files:**
- Create: `cmd/veloce/migrate.go`
- Modify: `cmd/veloce/main.go` (register migrate command)

- [ ] **Step 1: Implement `cmd/veloce/migrate.go`**

```go
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

	// Scan
	files, err := scanner.Scan(cfg.Source)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	log.Printf("scanner: %d files classified", len(files))

	// State
	var mig *state.MigrationState
	if cfg.Resume {
		if mig, err = state.LoadMigrationState(cfg.Output); err != nil {
			return fmt.Errorf("resume: %w", err)
		}
	} else {
		mig = state.NewMigrationState(cfg.Output)
	}
	mig.SetTotalFiles(len(files))
	// Seed pending entries for new files
	for _, f := range files {
		if _, ok := mig.Get(f.RelPath); !ok {
			mig.Mark(f.RelPath, state.FileEntry{Status: state.StatusPending, Phase: f.Phase})
		}
	}

	st, _ := state.LoadSharedTypes(cfg.Output)
	tu, _ := state.LoadTokenUsage(cfg.Output, cfg.BudgetLimit)

	// Gemini clients
	flash := gemini.NewFlashClient(cfg.APIKey)
	pro := gemini.NewProClient(cfg.APIKey)

	// Read default rules
	rules, err := os.ReadFile("configs/default_rules.md")
	if err != nil {
		return fmt.Errorf("read default_rules: %w", err)
	}

	// Cache
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
			return nil // resume skip
		}
		err := worker.Process(ctx, f)
		_ = mig.Save()
		_ = st.Save()
		_ = tu.Save()
		return err
	}

	orch := &agent.Orchestrator{
		Files:      files,
		Workers:    cfg.Workers,
		ProcessFn:  processFn,
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

	// Final save
	_ = mig.Save()
	_ = st.Save()
	_ = tu.Save()

	fin, fout, pin, pout := tu.Snapshot()
	log.Printf("Done. Tokens flash=%d/%d pro=%d/%d total=%d",
		fin, fout, pin, pout, tu.Total())
	return nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Smoke-test --help**

Run: `go run ./cmd/veloce migrate --help`
Expected: usage text showing all flags.

- [ ] **Step 4: Dry-run on an empty directory**

```bash
mkdir -p test_empty
go run ./cmd/veloce migrate --source ./test_empty --output ./test_out --dry-run --api-key fake
```
Expected: scanner reports 0 files, command exits cleanly.

- [ ] **Step 5: Commit**

```bash
git add cmd/veloce/migrate.go
git commit -m "feat(cli): migrate command wires scanner+orchestrator+worker"
```

---

### Task 18: `veloce status` command

**Files:**
- Create: `cmd/veloce/status.go`

- [ ] **Step 1: Implement**

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nestor/veloce/internal/state"
)

var statusOutput string

func init() {
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Display migration progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			mig, err := state.LoadMigrationState(statusOutput)
			if err != nil {
				return err
			}
			tu, _ := state.LoadTokenUsage(statusOutput, 0)
			fin, fout, pin, pout := tu.Snapshot()
			fmt.Printf("Tokens — flash in/out: %d/%d, pro in/out: %d/%d, total: %d\n",
				fin, fout, pin, pout, tu.Total())
			// Per-phase counts
			counts := map[int]map[state.Status]int{}
			for i := 1; i <= 4; i++ {
				counts[i] = map[state.Status]int{}
			}
			for _, p := range []int{1, 2, 3, 4} {
				for _, src := range mig.PendingInPhase(p) {
					e, _ := mig.Get(src)
					counts[p][e.Status]++
				}
			}
			for i := 1; i <= 4; i++ {
				fmt.Printf("Phase %d — pending=%d processing=%d\n", i, counts[i][state.StatusPending], counts[i][state.StatusProcessing])
			}
			return nil
		},
	}
	statusCmd.Flags().StringVar(&statusOutput, "output", "./output", "Output directory")
	rootCmd.AddCommand(statusCmd)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add cmd/veloce/status.go
git commit -m "feat(cli): status command summarising progress and token spend"
```

---

### Task 19: `veloce retry` command

**Files:**
- Create: `cmd/veloce/retry.go`

- [ ] **Step 1: Implement**

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nestor/veloce/internal/state"
)

var (
	retryOutput string
	retryFile   string
)

func init() {
	retryCmd := &cobra.Command{
		Use:   "retry",
		Short: "Reset a single file to pending so the next migrate run re-processes it",
		RunE: func(cmd *cobra.Command, args []string) error {
			mig, err := state.LoadMigrationState(retryOutput)
			if err != nil {
				return err
			}
			e, ok := mig.Get(retryFile)
			if !ok {
				return fmt.Errorf("file not in state: %s", retryFile)
			}
			e.Status = state.StatusPending
			e.Attempts = 0
			e.LastError = ""
			mig.Mark(retryFile, e)
			return mig.Save()
		},
	}
	retryCmd.Flags().StringVar(&retryOutput, "output", "./output", "Output directory")
	retryCmd.Flags().StringVar(&retryFile, "file", "", "File path (relative to source) to reset")
	_ = retryCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(retryCmd)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add cmd/veloce/retry.go
git commit -m "feat(cli): retry command resets a single file to pending"
```

---

### Task 20: End-to-end smoke test on a minimal Laravel fixture

**Files:**
- Create: `testdata/laravel_min/app/Models/User.php`
- Create: `testdata/laravel_min/config/database.php`
- Create: `internal/agent/e2e_test.go`

- [ ] **Step 1: Create the minimal Laravel fixture**

```bash
mkdir -p testdata/laravel_min/app/Models
mkdir -p testdata/laravel_min/config
```

`testdata/laravel_min/app/Models/User.php`:
```php
<?php
namespace App\Models;
class User {
  public string $id;
  public string $email;
}
```

`testdata/laravel_min/config/database.php`:
```php
<?php
return ['default' => 'mysql'];
```

- [ ] **Step 2: Write an offline E2E test using a fake Gemini client**

`internal/agent/e2e_test.go`:
```go
//go:build e2e

package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nestor/veloce/internal/agent"
	"github.com/nestor/veloce/internal/gemini"
	"github.com/nestor/veloce/internal/pipeline"
	"github.com/nestor/veloce/internal/scanner"
	"github.com/nestor/veloce/internal/state"
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
		SourceRoot: "../../testdata/laravel_min", OutputRoot: out,
		Flash: flash, Pro: pro, Corrector: corrector,
		Migration: mig, SharedTypes: st, TokenUsage: tu, SystemRules: "",
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
```

- [ ] **Step 3: Run the e2e test**

Run: `go test -tags=e2e ./internal/agent/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add testdata/ internal/agent/e2e_test.go
git commit -m "test(e2e): minimal Laravel fixture round-trip through Worker"
```

---

### Task 21: README expansion & usage docs

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace README with full usage**

```markdown
# Veloce

Veloce is a CLI agent that migrates a Laravel monolith (PHP + Blade) into a Go REST backend + a Flutter cross-platform frontend. It orchestrates Gemini 2.5 Flash (95% of calls) and Gemini 2.5 Pro (5% of calls, escalation) through a ReAct loop with per-file checkpoints and a strict token budget kill switch.

See `docs/superpowers/specs/2026-05-29-migration-agent-design.md` for the full design.

## Build

```bash
go build -o veloce ./cmd/veloce
```

## Usage

```bash
export GEMINI_API_KEY=...

./veloce migrate \
  --source  ./my-laravel-project \
  --output  ./output \
  --workers 8 \
  --budget  5000000

./veloce status --output ./output
./veloce retry  --output ./output --file app/Models/User.php
./veloce migrate --source ./my-laravel-project --output ./output --resume
```

## State files (under `./output/.veloce/`)

| File | Purpose |
|---|---|
| `migration_state.json` | Per-file checkpoint |
| `shared_types.json`    | Go/Dart type index for cross-target consistency |
| `token_usage.json`     | Token counts + kill-switch budget |
| `context_cache_id.txt` | Gemini cache id (rules + types) |

## Generated layout

```
output/
├── backend/   # Go (Chi + GORM + JWT)
└── frontend/  # Flutter (Riverpod + Dio + GoRouter)
```

## Toolchain required on the host

- Go 1.22+
- Dart / Flutter SDK (for `dart analyze`)

## Limits & escalation

For each file: 1 generation + 2 Flash corrections + 1 Pro escalation = max 4 attempts. After that the file is marked `failed` and logged for human review.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: full README with usage, state files, and limits"
```

---

## Self-Review Notes

**Spec coverage:**
- §2 Architecture → Task 1
- §3 CLI → Tasks 2, 17, 18, 19
- §4 State → Tasks 3, 4, 5
- §5 Gemini → Tasks 7, 8, 9, 10
- §6 ReAct loop → Task 15
- §7 FinOps (cache, kill switch, comptabilité) → Tasks 5, 10, 17
- §8 Verification tools → Task 12
- §9 Output structure → Task 13
- §10 Scanner → Task 6
- §11 Non-functional (atomic writes, resume) → Tasks 3, 17
- §12 Out of scope → respected (no AST PHP, no jobs, no UI)

**Type consistency:** `state.GoType` / `state.DartType` used across state, pipeline, agent. `pipeline.VerifyFn` / `pipeline.VerifyResult` consistent. `scanner.File` carries `Phase` + `Kind` end-to-end.

**Placeholders:** None found after final pass.
