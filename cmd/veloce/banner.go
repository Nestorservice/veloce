package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cDim    = "\x1b[2m"
	cCyan   = "\x1b[38;5;51m"
	cBlue   = "\x1b[38;5;39m"
	cPurple = "\x1b[38;5;141m"
	cPink   = "\x1b[38;5;213m"
	cGreen  = "\x1b[38;5;120m"
	cYellow = "\x1b[38;5;227m"
	cGray   = "\x1b[38;5;245m"
	cWhite  = "\x1b[38;5;255m"
)

const version = "0.1.0"

var asciiLogo = []string{
	"  ██╗   ██╗███████╗██╗      ██████╗  ██████╗███████╗",
	"  ██║   ██║██╔════╝██║     ██╔═══██╗██╔════╝██╔════╝",
	"  ██║   ██║█████╗  ██║     ██║   ██║██║     █████╗  ",
	"  ╚██╗ ██╔╝██╔══╝  ██║     ██║   ██║██║     ██╔══╝  ",
	"   ╚████╔╝ ███████╗███████╗╚██████╔╝╚██████╗███████╗",
	"    ╚═══╝  ╚══════╝╚══════╝ ╚═════╝  ╚═════╝╚══════╝",
}

// gradient picks a color along a blue→purple→pink gradient based on index/total.
func gradient(i, n int) string {
	switch {
	case i < n/3:
		return cCyan
	case i < 2*n/3:
		return cPurple
	default:
		return cPink
	}
}

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func paint(c, s string) string {
	if !colorEnabled() {
		return s
	}
	return c + s + cReset
}

// PrintSplash renders the welcome banner. Called once at the start of every veloce invocation.
func PrintSplash() {
	w := os.Stdout
	fmt.Fprintln(w)
	for i, line := range asciiLogo {
		fmt.Fprintln(w, paint(gradient(i, len(asciiLogo))+cBold, line))
	}
	fmt.Fprintln(w)

	tagline := "Laravel  →  Go + Flutter  migration agent"
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, tagline))

	sub := "Powered by OpenRouter  ·  Batch processing  ·  Per-file checkpoints"
	fmt.Fprintln(w, "  "+paint(cGray, sub))
	fmt.Fprintln(w)

	rule := strings.Repeat("─", 56)
	fmt.Fprintln(w, "  "+paint(cDim+cBlue, rule))
	fmt.Fprintln(w, "  "+paint(cYellow+cBold, "  ⚓  Developed by OceanTechnologie"))
	fmt.Fprintln(w, "  "+paint(cDim+cGray, fmt.Sprintf("     v%s  ·  github.com/Nestorservice/veloce", version)))
	fmt.Fprintln(w, "  "+paint(cDim+cBlue, rule))
	fmt.Fprintln(w)
}

// PrintRunHeader renders the per-run summary box (source/output/files/budget).
func PrintRunHeader(source, output string, files int, breakdown string, budget int, dryRun bool) {
	w := os.Stdout
	top := "┌─ " + paint(cCyan+cBold, "Veloce run") + " " + strings.Repeat("─", 45) + "┐"
	fmt.Fprintln(w, top)
	fmt.Fprintf(w, "│ %s %s\n", paint(cGray, "Source :"), paint(cWhite, source))
	fmt.Fprintf(w, "│ %s %s\n", paint(cGray, "Output :"), paint(cGreen, output))
	fmt.Fprintf(w, "│ %s %s  %s\n", paint(cGray, "Files  :"), paint(cWhite, fmt.Sprintf("%d", files)), paint(cDim+cGray, "("+breakdown+")"))
	fmt.Fprintf(w, "│ %s %s tokens  %s\n",
		paint(cGray, "Budget :"),
		paint(cWhite, fmt.Sprintf("%d", budget)),
		paint(cDim+cGray, fmt.Sprintf("(~$%.2f max)", float64(budget)*0.075/1_000_000)),
	)
	if dryRun {
		fmt.Fprintf(w, "│ %s %s\n", paint(cGray, "Mode   :"), paint(cYellow+cBold, "DRY-RUN — no API calls, no writes"))
	}
	fmt.Fprintln(w, "└"+strings.Repeat("─", 58)+"┘")
}

// PrintMissingAPIKeyHint is shown when the OpenRouter API key is not provided.
func PrintMissingAPIKeyHint() {
	w := os.Stdout
	fmt.Fprintln(w, "  "+paint(cYellow, "ℹ  No OpenRouter API key found in this session."))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "1. Get a free key (no credit card needed):"))
	fmt.Fprintln(w, "    "+paint(cCyan, "https://openrouter.ai/keys"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "2. Set it for this PowerShell session:"))
	fmt.Fprintln(w, "    "+paint(cGreen, `$env:OPENROUTER_API_KEY = "sk-or-..."`))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "   Or make it permanent (Windows):"))
	fmt.Fprintln(w, "    "+paint(cGreen, `setx OPENROUTER_API_KEY "sk-or-..."`)+paint(cDim+cGray, "  # open a new terminal after"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "   Or add it to your project .env:"))
	fmt.Fprintln(w, "    "+paint(cGreen, `OPENROUTER_API_KEY=sk-or-...`)+paint(cDim+cGray, "   # inside your Laravel project"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "   Or pass it directly:"))
	fmt.Fprintln(w, "    "+paint(cGreen, "veloce --api-key sk-or-..."))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cDim+cGray, "To analyze without any API call: ")+paint(cCyan, "veloce --dry-run"))
	fmt.Fprintln(w)
}

// PrintHelpfulHint is shown when invoked outside a Laravel project.
func PrintHelpfulHint() {
	w := os.Stdout
	fmt.Fprintln(w, "  "+paint(cYellow, "ℹ  No Laravel project detected in this directory."))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "Quick start:"))
	fmt.Fprintln(w, "    "+paint(cGreen, "cd ")+paint(cWhite, "/path/to/your-laravel-project"))
	fmt.Fprintln(w, "    "+paint(cGreen, "veloce")+paint(cDim+cGray, "                # auto: source=.  output=../<name>_output"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "Useful flags:"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce --dry-run")+paint(cDim+cGray, "       # analyze only, zero API calls"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce --dry-run")+paint(cDim+cGray, "                    # analyze only, zero API calls"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce --batch-size 10 --delay 5")+paint(cDim+cGray, "  # smaller batches, shorter pause"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce status")+paint(cDim+cGray, "                      # progress for current run"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce --help")+paint(cDim+cGray, "                      # all options"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cDim+cGray, "Need a free OpenRouter key?  https://openrouter.ai/keys"))
	fmt.Fprintln(w)
}
