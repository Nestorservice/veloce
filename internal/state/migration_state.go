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
