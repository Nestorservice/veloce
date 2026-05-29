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
