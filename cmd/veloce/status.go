package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Nestorservice/veloce/internal/state"
)

// defaultOutputDir returns <cwd>_output (sibling of cwd) when --output is empty.
func defaultOutputDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "./output"
	}
	return filepath.Join(filepath.Dir(cwd), filepath.Base(cwd)+"_output")
}

var statusOutput string

func init() {
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Display migration progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := defaultOutputDir(statusOutput)
			mig, err := state.LoadMigrationState(out)
			if err != nil {
				return fmt.Errorf("no migration state in %s — run `veloce` first", out)
			}
			tu, _ := state.LoadTokenUsage(out, 0)
			fin, fout, pin, pout := tu.Snapshot()
			fmt.Printf("Tokens — flash in/out: %d/%d, pro in/out: %d/%d, total: %d\n",
				fin, fout, pin, pout, tu.Total())
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
	statusCmd.Flags().StringVar(&statusOutput, "output", "", "Output directory (default: <cwd>_output)")
	rootCmd.AddCommand(statusCmd)
}
