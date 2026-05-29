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
