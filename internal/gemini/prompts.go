package gemini

import (
	"fmt"
	"strings"
)

type TranslationRequest struct {
	Target      string // "go" | "dart"
	PhaseKind   string // "config", "model", "controller", "blade", ...
	SourcePath  string
	SourceCode  string
	SharedTypes string
	ArchHint    string
}

type CorrectionRequest struct {
	Target       string
	PreviousCode string
	BuildError   string
}

func BuildTranslationPrompt(r TranslationRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Existing types\n%s\n\n", r.SharedTypes)
	fmt.Fprintf(&b, "## Translation request\n")
	fmt.Fprintf(&b, "- Target language: %s\n", r.Target)
	fmt.Fprintf(&b, "- Source file: %s\n", r.SourcePath)
	fmt.Fprintf(&b, "- Kind: %s\n", r.PhaseKind)
	fmt.Fprintf(&b, "- Architecture hint: %s\n\n", r.ArchHint)
	fmt.Fprintf(&b, "## PHP source\n```php\n%s\n```\n\n", r.SourceCode)
	fmt.Fprintf(&b, "Respond with ONLY valid %s code. No prose, no markdown.\n", strings.ToUpper(r.Target))
	return b.String()
}

func BuildCorrectionPrompt(r CorrectionRequest) string {
	var b strings.Builder
	b.WriteString("## Previous attempt failed to compile.\n\n")
	fmt.Fprintf(&b, "### Build error\n```\n%s\n```\n\n", r.BuildError)
	fmt.Fprintf(&b, "### Previous code\n```%s\n%s\n```\n\n", r.Target, r.PreviousCode)
	fmt.Fprintf(&b, "Produce a corrected version. Respond with ONLY valid %s code. No prose.\n", strings.ToUpper(r.Target))
	return b.String()
}
