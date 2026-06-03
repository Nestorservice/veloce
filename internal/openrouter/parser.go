package openrouter

import (
	"strings"
)

// ParsedFile is a single output file extracted from the model's response.
type ParsedFile struct {
	Path    string // output path as emitted by the model (e.g. "backend/internal/domain/user.go")
	Content string // full file content
	Lang    string // "go" or "dart"
}

// ParseBatchResponse extracts all output files from a model response.
// The model is expected to emit blocks like:
//
//	=== GO: path/to/file.go ===
//	<content>
//
//	=== DART: path/to/file.dart ===
//	<content>
//
// Blocks end when the next delimiter is found or the text ends.
// Content that appears before the first delimiter is silently discarded.
func ParseBatchResponse(response string) []ParsedFile {
	lines := strings.Split(response, "\n")
	var files []ParsedFile

	var current *ParsedFile
	var contentLines []string

	flush := func() {
		if current == nil {
			return
		}
		current.Content = strings.TrimRight(strings.Join(contentLines, "\n"), "\n") + "\n"
		files = append(files, *current)
		current = nil
		contentLines = nil
	}

	for _, line := range lines {
		if path, lang, ok := parseDelimiter(line); ok {
			flush()
			current = &ParsedFile{Path: path, Lang: lang}
			contentLines = nil
			continue
		}
		if current != nil {
			contentLines = append(contentLines, line)
		}
	}
	flush()

	return files
}

// parseDelimiter checks whether line is a file delimiter and returns the path
// and language if so.
//
//	"=== GO: internal/handler/user_handler.go ==="   → ("internal/handler/user_handler.go", "go", true)
//	"=== DART: lib/features/auth/login_screen.dart ===" → (..., "dart", true)
func parseDelimiter(line string) (path, lang string, ok bool) {
	line = strings.TrimSpace(line)
	for _, prefix := range []struct {
		marker string
		lang   string
	}{
		{"=== GO: ", "go"},
		{"=== DART: ", "dart"},
	} {
		if strings.HasPrefix(line, prefix.marker) && strings.HasSuffix(line, " ===") {
			inner := strings.TrimPrefix(line, prefix.marker)
			inner = strings.TrimSuffix(inner, " ===")
			inner = strings.TrimSpace(inner)
			if inner != "" {
				return inner, prefix.lang, true
			}
		}
	}
	return "", "", false
}

// PathHint supplies the expected output path and language for one source file.
// Used by ParseMarkdownFallback so it can assign extracted code blocks to the
// correct output paths even when the model omitted the delimiter headers.
type PathHint struct {
	Path string // expected output path, e.g. "backend/internal/config/app.go"
	Lang string // "go", "dart", or "sql"
}

// ParseMarkdownFallback is called when ParseBatchResponse finds no delimiters.
// It extracts code from markdown fences (` ```go ... ``` `) and pairs each
// block with the corresponding hint.  Path comments in the first line of the
// code block ("// backend/...") take precedence over the hint path.
func ParseMarkdownFallback(response string, hints []PathHint) []ParsedFile {
	type block struct {
		lang     string
		pathHint string
		code     string
	}

	var blocks []block
	lines := strings.Split(response, "\n")
	inBlock := false
	var curLang, curPathHint string
	var curLines []string

	for _, line := range lines {
		if !inBlock {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				fence := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
				// Accept go, dart, sql, or unlabelled fences.
				if fence == "go" || fence == "dart" || fence == "sql" || fence == "" {
					inBlock = true
					curLang = fence
					curPathHint = ""
					curLines = nil
				}
			}
		} else {
			if strings.TrimSpace(line) == "```" {
				// Detect a path hint in the first code line (e.g. "// backend/...")
				if len(curLines) > 0 {
					first := strings.TrimSpace(curLines[0])
					if strings.HasPrefix(first, "//") || strings.HasPrefix(first, "--") {
						candidate := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(first, "--"), "//"))
						if strings.Contains(candidate, "/") &&
							(strings.HasSuffix(candidate, ".go") ||
								strings.HasSuffix(candidate, ".dart") ||
								strings.HasSuffix(candidate, ".sql")) {
							curPathHint = candidate
							curLines = curLines[1:] // strip the comment line
						}
					}
				}
				code := strings.TrimRight(strings.Join(curLines, "\n"), "\n") + "\n"
				if strings.TrimSpace(code) != "" {
					blocks = append(blocks, block{lang: curLang, pathHint: curPathHint, code: code})
				}
				inBlock = false
			} else {
				curLines = append(curLines, line)
			}
		}
	}

	// Pair blocks with hints.
	var files []ParsedFile
	for i, blk := range blocks {
		if strings.TrimSpace(blk.code) == "" {
			continue
		}
		path := blk.pathHint
		lang := blk.lang
		if i < len(hints) {
			if path == "" {
				path = hints[i].Path
			}
			if lang == "" {
				lang = hints[i].Lang
			}
		}
		if path == "" || lang == "" {
			continue
		}
		files = append(files, ParsedFile{Path: path, Content: blk.code, Lang: lang})
	}
	return files
}

// ValidateParsedFiles checks that paths are non-empty and content is non-trivial.
// Returns a slice of error strings (empty if all files look valid).
func ValidateParsedFiles(files []ParsedFile) []string {
	var errs []string
	for _, f := range files {
		if f.Path == "" {
			errs = append(errs, "empty output path")
			continue
		}
		if len(strings.TrimSpace(f.Content)) < 10 {
			errs = append(errs, "nearly-empty content for "+f.Path)
		}
		switch f.Lang {
		case "go":
			if !strings.Contains(f.Content, "package ") {
				errs = append(errs, "Go file missing package declaration: "+f.Path)
			}
		case "dart":
			// Dart files don't need a specific required keyword, but they should
			// contain something substantive.
		default:
			errs = append(errs, "unknown language "+f.Lang+" for "+f.Path)
		}
	}
	return errs
}
