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

	sub := "Powered by Gemini 2.5  ·  ReAct loop  ·  Per-file checkpoints"
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

// PrintMissingAPIKeyHint is shown when the Gemini API key is not provided.
func PrintMissingAPIKeyHint() {
	w := os.Stdout
	fmt.Fprintln(w, "  "+paint(cYellow, "ℹ  No Gemini API key found in this session."))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "Set it for this PowerShell session:"))
	fmt.Fprintln(w, "    "+paint(cGreen, `$env:GEMINI_API_KEY = "AIza..."`))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "Or make it permanent (Windows):"))
	fmt.Fprintln(w, "    "+paint(cGreen, `setx GEMINI_API_KEY "AIza..."`)+paint(cDim+cGray, "   # then open a new terminal"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cWhite+cBold, "Or pass it directly:"))
	fmt.Fprintln(w, "    "+paint(cGreen, "veloce --api-key AIza..."))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cDim+cGray, "Get a key: https://aistudio.google.com/apikey"))
	fmt.Fprintln(w, "  "+paint(cDim+cGray, "Or skip the API entirely with: ")+paint(cCyan, "veloce --dry-run"))
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
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce --budget 500000")+paint(cDim+cGray, " # cap tokens (~$0.04)"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce status")+paint(cDim+cGray, "          # progress for current run"))
	fmt.Fprintln(w, "    "+paint(cCyan, "veloce --help")+paint(cDim+cGray, "          # all options"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+paint(cDim+cGray, "Need a Gemini API key?  https://aistudio.google.com/apikey"))
	fmt.Fprintln(w)
}
