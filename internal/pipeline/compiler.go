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
