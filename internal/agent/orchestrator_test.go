package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nestor/veloce/internal/scanner"
)

func TestOrchestrator_RunsPhasesSequentially(t *testing.T) {
	var phase2Done atomic.Bool

	proc := func(ctx context.Context, f scanner.File) error {
		if f.Phase == 3 && !phase2Done.Load() {
			t.Error("phase 3 file processed before phase 2 finished")
		}
		return nil
	}

	files := []scanner.File{
		{RelPath: "a", Phase: 2}, {RelPath: "b", Phase: 2},
		{RelPath: "c", Phase: 3}, {RelPath: "d", Phase: 3},
	}

	o := &Orchestrator{Files: files, Workers: 2, ProcessFn: proc, OnPhaseEnd: func(p int) {
		if p == 2 {
			phase2Done.Store(true)
		}
	}}
	if err := o.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
