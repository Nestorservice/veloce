package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "veloce",
	Short: "Veloce — Laravel → Go+Flutter migration agent",
	Long: `Veloce migrates a Laravel project to Go + Flutter.

Run with no arguments from inside a Laravel project: it auto-detects the
project, writes the result to <project>_output/ next to it, and resumes
from the last checkpoint if you re-run.`,
	RunE:          runMigrate,
	SilenceUsage:  true,
	SilenceErrors: false,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
