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
