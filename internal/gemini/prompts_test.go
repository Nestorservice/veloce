package gemini

import (
	"strings"
	"testing"
)

func TestBuildTranslationPrompt_IncludesAllParts(t *testing.T) {
	p := BuildTranslationPrompt(TranslationRequest{
		Target:      "go",
		PhaseKind:   "model",
		SourcePath:  "app/Models/User.php",
		SourceCode:  "<?php class User extends Model {}",
		SharedTypes: "type Order struct { ID uuid.UUID }",
		ArchHint:    "Generate a domain struct in package domain.",
	})
	for _, want := range []string{"go", "model", "User.php", "Order", "domain", "ONLY"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildCorrectionPrompt_IncludesError(t *testing.T) {
	p := BuildCorrectionPrompt(CorrectionRequest{
		Target:       "go",
		PreviousCode: "package domain\ntype User struct { ID UUID }",
		BuildError:   "undefined: UUID",
	})
	if !strings.Contains(p, "undefined: UUID") {
		t.Errorf("correction prompt missing build error")
	}
	if !strings.Contains(p, "ONLY") {
		t.Errorf("correction prompt missing strict-output directive")
	}
}
