package agent

import (
	"context"
	"sort"
	"sync"

	"github.com/Nestorservice/veloce/internal/scanner"
)

type ProcessFn func(ctx context.Context, f scanner.File) error

type Orchestrator struct {
	Files       []scanner.File
	Workers     int
	ProcessFn   ProcessFn
	OnPhaseEnd  func(phase int)
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
