package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Nestorservice/veloce/internal/state"
)

var (
	retryOutput string
	retryFile   string
)

func init() {
	retryCmd := &cobra.Command{
		Use:   "retry",
		Short: "Reset a single file to pending so the next migrate run re-processes it",
		RunE: func(cmd *cobra.Command, args []string) error {
			mig, err := state.LoadMigrationState(retryOutput)
			if err != nil {
				return err
			}
			e, ok := mig.Get(retryFile)
			if !ok {
				return fmt.Errorf("file not in state: %s", retryFile)
			}
			e.Status = state.StatusPending
			e.Attempts = 0
			e.LastError = ""
			mig.Mark(retryFile, e)
			return mig.Save()
		},
	}
	retryCmd.Flags().StringVar(&retryOutput, "output", "./output", "Output directory")
	retryCmd.Flags().StringVar(&retryFile, "file", "", "File path (relative to source) to reset")
	_ = retryCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(retryCmd)
}
