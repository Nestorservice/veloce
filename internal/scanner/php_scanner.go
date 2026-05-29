package scanner

import (
	"io/fs"
	"path/filepath"
	"strings"
)

type File struct {
	AbsPath string
	RelPath string // relative to project root
	Phase   int
	Kind    string // "config", "route", "migration", "model", "controller", "service", "request", "blade"
}

// Scan walks the Laravel root and returns a classified list of files.
// Vendor, storage, bootstrap/cache, node_modules and tests are skipped.
func Scan(root string) ([]File, error) {
	var out []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if isSkipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".php") {
			return nil
		}

		phase, kind, ok := classify(rel)
		if !ok {
			return nil
		}
		out = append(out, File{AbsPath: path, RelPath: rel, Phase: phase, Kind: kind})
		return nil
	})
	return out, err
}

func isSkipDir(rel string) bool {
	switch rel {
	case "vendor", "storage", "node_modules", "bootstrap/cache", "tests", ".git":
		return true
	}
	return false
}

func classify(rel string) (int, string, bool) {
	switch {
	case strings.HasPrefix(rel, "config/"):
		return 1, "config", true
	case strings.HasPrefix(rel, "routes/"):
		return 1, "route", true
	case strings.HasPrefix(rel, "database/migrations/"):
		return 1, "migration", true
	case strings.HasPrefix(rel, "app/Models/"), strings.HasPrefix(rel, "app/Repositories/"):
		return 2, "model", true
	case strings.HasPrefix(rel, "app/Http/Controllers/"):
		return 3, "controller", true
	case strings.HasPrefix(rel, "app/Services/"):
		return 3, "service", true
	case strings.HasPrefix(rel, "app/Http/Requests/"):
		return 3, "request", true
	case strings.HasPrefix(rel, "resources/views/") && strings.HasSuffix(rel, ".blade.php"):
		return 4, "blade", true
	}
	return 0, "", false
}
