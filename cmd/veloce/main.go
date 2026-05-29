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
	SilenceErrors: true,
}

func main() {
	if shouldShowSplash() {
		PrintSplash()
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, paint(cPink+cBold, "✗ "+err.Error()))
		os.Exit(1)
	}
}

// Splash shown for the root command and `migrate`, but not for status/retry/help/completion etc.
func shouldShowSplash() bool {
	if len(os.Args) < 2 {
		return true
	}
	for _, a := range os.Args[1:] {
		switch a {
		case "-h", "--help", "help", "completion", "__complete":
			return false
		case "status", "retry":
			return false
		}
		if a == "migrate" {
			return true
		}
	}
	return true
}
