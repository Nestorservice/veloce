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
