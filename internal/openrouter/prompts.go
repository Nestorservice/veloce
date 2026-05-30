package openrouter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nestorservice/veloce/internal/batcher"
)

// ---- delimiter constants ---------------------------------------------------

// InputDelim wraps each source PHP file in the batch prompt.
const InputDelim = "=== PHP: %s ==="

// OutputGoDelim is what the model must emit for each generated Go file.
const OutputGoDelim = "=== GO: %s ==="

// OutputDartDelim is what the model must emit for each generated Dart/Flutter file.
const OutputDartDelim = "=== DART: %s ==="

// ---- system prompt ---------------------------------------------------------

// SystemPrompt returns the immutable instruction block sent as the "system"
// message before every batch. It tells the model exactly what format it must
// use so the parser can reliably extract files.
func SystemPrompt(rules string) string {
	var b strings.Builder
	b.WriteString(rules)
	b.WriteString(`

## BATCH TRANSLATION FORMAT

You will receive one or more PHP source files delimited like this:

=== PHP: path/to/file.php ===
<file content>

For EACH input file you MUST emit EXACTLY ONE output block using EITHER:
  • Go backend file   → === GO: path/to/output.go ===
  • Dart/Flutter file → === DART: path/to/output.dart ===

Rules:
1. Emit ALL output blocks BEFORE any explanation text.
2. Use the exact delimiters shown above — no extra spaces, no markdown fences
   around the delimiters themselves (code inside may use fences if needed).
3. Infer the correct output path from the input path:
   • app/Models/Foo.php          → internal/domain/foo.go
   • app/Http/Controllers/X.php  → internal/handler/x_handler.go
   • app/Http/Requests/X.php     → internal/handler/x_request.go
   • resources/views/x.blade.php → lib/features/<feature>/presentation/screens/x_screen.dart
   • config/foo.php              → internal/config/foo.go
   • database/migrations/xxx.php → migrations/xxx.sql
   • routes/*.php                → internal/router/routes.go  (merge all routes)
4. Generate idiomatic Go (1.22+) or idiomatic Dart/Flutter (3.x).
5. Never include TODO, placeholder, or partial implementations.
6. If a file only exports types, emit a Go struct file in internal/domain/.
`)
	return b.String()
}

// ---- batch prompt builder --------------------------------------------------

// BuildBatchPrompt creates the user message for a single batch. Each source
// file is delimited so the model knows where one file ends and the next begins.
func BuildBatchPrompt(batch batcher.Batch) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Translate the following %d PHP/Blade file(s) to Go + Flutter.\n\n", len(batch.Files))
	for _, f := range batch.Files {
		fmt.Fprintf(&b, "=== PHP: %s ===\n", f.RelPath)
		b.WriteString(f.Content)
		if !strings.HasSuffix(f.Content, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// ---- output path helpers ---------------------------------------------------

// GoOutputPath derives the Go output path from a PHP source path.
// This is the canonical mapping; the model should follow the same rules via
// the system prompt, but we also compute it locally for verification.
func GoOutputPath(rel string) string {
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasPrefix(rel, "app/Models/") || strings.HasPrefix(rel, "app/Entities/"):
		base := stem(rel)
		return "backend/internal/domain/" + snakeCase(base) + ".go"
	case strings.HasPrefix(rel, "app/Http/Controllers/"):
		base := stem(rel)
		return "backend/internal/handler/" + snakeCase(base) + "_handler.go"
	case strings.HasPrefix(rel, "app/Http/Requests/"):
		base := stem(rel)
		return "backend/internal/handler/" + snakeCase(base) + "_request.go"
	case strings.HasPrefix(rel, "app/Services/"):
		base := stem(rel)
		return "backend/internal/service/" + snakeCase(base) + ".go"
	case strings.HasPrefix(rel, "app/Repositories/"):
		base := stem(rel)
		return "backend/internal/repository/" + snakeCase(base) + ".go"
	case strings.HasPrefix(rel, "config/"):
		base := stem(rel)
		return "backend/internal/config/" + snakeCase(base) + ".go"
	case strings.HasPrefix(rel, "database/migrations/"):
		base := stem(rel)
		return "backend/migrations/" + base + ".sql"
	case strings.HasPrefix(rel, "routes/"):
		return "backend/internal/router/routes.go"
	case strings.HasPrefix(rel, "Modules/"):
		// Modules/<Name>/<subpath> → recurse on subpath.
		parts := strings.SplitN(rel, "/", 3)
		if len(parts) == 3 {
			return GoOutputPath(parts[2])
		}
	}
	// Fallback: keep relative path under backend/internal/misc/.
	base := stem(rel)
	return "backend/internal/misc/" + snakeCase(base) + ".go"
}

// DartOutputPath derives the Flutter output path from a Blade source path.
func DartOutputPath(rel string) string {
	rel = filepath.ToSlash(rel)
	// Strip resources/views/ prefix if present.
	rel = strings.TrimPrefix(rel, "resources/views/")
	rel = strings.TrimSuffix(rel, ".blade.php")
	rel = strings.TrimSuffix(rel, ".php")

	// Derive feature name from first directory segment.
	parts := strings.SplitN(rel, "/", 2)
	feature := snakeCase(parts[0])
	screen := snakeCase(filepath.Base(rel))
	if !strings.HasSuffix(screen, "_screen") {
		screen += "_screen"
	}
	return fmt.Sprintf("frontend/lib/features/%s/presentation/screens/%s.dart", feature, screen)
}

// ---- small string helpers --------------------------------------------------

// stem returns the filename without extension.
func stem(rel string) string {
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// snakeCase converts CamelCase / PascalCase to snake_case.
// "ProductController" → "product_controller"
// Already-snake strings are returned unchanged.
func snakeCase(s string) string {
	var out strings.Builder
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			if i > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(r + 32) // toLower
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			out.WriteRune(r)
		default:
			// replace any other char (dash, dot, space…) with underscore
			out.WriteByte('_')
		}
	}
	result := out.String()
	// Collapse consecutive underscores.
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}
	return strings.Trim(result, "_")
}
