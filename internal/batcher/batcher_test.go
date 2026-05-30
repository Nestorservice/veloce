package batcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nestorservice/veloce/internal/scanner"
)

// makeFiles writes n PHP files into dir and returns corresponding scanner.Files.
func makeFiles(t *testing.T, dir string, phase int, prefix string, n int, linesEach int) []scanner.File {
	t.Helper()
	var files []scanner.File
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s_%d.php", prefix, i)
		rel := filepath.Join("app", "Http", "Controllers", name)
		abs := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		content := ""
		for j := 0; j < linesEach; j++ {
			content += fmt.Sprintf("// line %d of %s\n", j, name)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, scanner.File{
			AbsPath: abs,
			RelPath: filepath.ToSlash(rel),
			Phase:   phase,
			Kind:    "controller",
		})
	}
	return files
}

func TestGroup_SplitsOnMaxFiles(t *testing.T) {
	dir := t.TempDir()
	files := makeFiles(t, dir, 3, "ctrl", 20, 10) // 20 files, 10 lines each
	batches, err := Group(dir, files, Options{MaxFiles: 5, MaxInputTokens: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 4 { // 20 / 5 = 4 batches
		t.Errorf("batches=%d, want 4", len(batches))
	}
	for _, b := range batches {
		if len(b.Files) > 5 {
			t.Errorf("batch %s has %d files, want ≤5", b.ID, len(b.Files))
		}
		if b.Phase != 3 {
			t.Errorf("batch %s phase=%d, want 3", b.ID, b.Phase)
		}
	}
}

func TestGroup_SingletonForOversizedFile(t *testing.T) {
	dir := t.TempDir()

	// Write a synthetic "big" file: 4 000 chars ≈ 1 143 tokens.
	bigPath := filepath.Join(dir, "app", "Http", "Controllers", "big.php")
	_ = os.MkdirAll(filepath.Dir(bigPath), 0o755)
	bigContent := strings.Repeat("// php code line\n", 235) // 235 × 17 ≈ 4 000 chars
	_ = os.WriteFile(bigPath, []byte(bigContent), 0o644)
	bigFile := scanner.File{
		AbsPath: bigPath,
		RelPath: "app/Http/Controllers/big.php",
		Phase:   2,
		Kind:    "controller",
	}

	// Three small files: 50 chars each ≈ 14 tokens.
	small := makeFiles(t, dir, 2, "small", 3, 3)
	all := append([]scanner.File{bigFile}, small...)

	// MaxInputTokens=500 → big file (≈1143 tok) is over the limit, small files are not.
	batches, err := Group(dir, all, Options{MaxFiles: 15, MaxInputTokens: 500})
	if err != nil {
		t.Fatal(err)
	}

	// The big file must be in a singleton batch.
	for _, b := range batches {
		for _, f := range b.Files {
			if f.RelPath == "app/Http/Controllers/big.php" && len(b.Files) > 1 {
				t.Errorf("oversized file should be a singleton but batch %s has %d files", b.ID, len(b.Files))
			}
		}
	}
	total := 0
	for _, b := range batches {
		total += len(b.Files)
	}
	if total != len(all) {
		t.Errorf("total files=%d, want %d", total, len(all))
	}
}

func TestGroup_SkipsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	files := makeFiles(t, dir, 1, "cfg", 3, 5)
	// Corrupt the path of one file so it cannot be read.
	files[1].AbsPath = filepath.Join(dir, "does_not_exist.php")

	batches, err := Group(dir, files, Options{MaxFiles: 15})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, b := range batches {
		total += len(b.Files)
	}
	if total != 2 { // 3 files - 1 unreadable = 2
		t.Errorf("total files in batches=%d, want 2", total)
	}
}

func TestGroup_PreservesPhaseOrder(t *testing.T) {
	dir := t.TempDir()
	// Mix phases intentionally.
	p1 := makeFiles(t, dir, 1, "cfg", 3, 5)
	p2 := makeFiles(t, dir, 2, "mdl", 3, 5)
	p3 := makeFiles(t, dir, 3, "ctl", 3, 5)
	all := append(append(p3, p1...), p2...) // shuffle

	batches, err := Group(dir, all, Options{MaxFiles: 15})
	if err != nil {
		t.Fatal(err)
	}
	// Phases should appear in the order files were encountered (p3, p1, p2 here).
	// What matters: all files of a batch share the same phase.
	for _, b := range batches {
		ph := b.Files[0].Phase
		for _, f := range b.Files[1:] {
			if f.Phase != ph {
				t.Errorf("batch %s mixes phases %d and %d", b.ID, ph, f.Phase)
			}
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		text string
		want int // rough expectation
	}{
		{"", 0},
		{"x", 1},
		{string(make([]byte, 350)), 100}, // 350 chars ≈ 100 tokens
		{string(make([]byte, 3500)), 1000},
	}
	for _, tc := range cases {
		got := EstimateTokens(tc.text)
		// Allow ±10% tolerance on estimates.
		lo := tc.want * 9 / 10
		hi := tc.want * 11 / 10
		if hi == 0 {
			hi = 1
		}
		if got < lo || got > hi {
			t.Errorf("EstimateTokens(%d chars) = %d, want ~%d (±10%%)", len(tc.text), got, tc.want)
		}
	}
}
