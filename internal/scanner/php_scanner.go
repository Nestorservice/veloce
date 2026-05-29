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
	case "vendor", "storage", "node_modules", "bootstrap/cache", "tests", ".git", "public", "packages", "xampp", "webalizer":
		return true
	}
	// Skip vendor / node_modules anywhere in the tree (nested modules)
	if strings.Contains(rel, "/vendor") || strings.Contains(rel, "/node_modules") {
		return true
	}
	return false
}

// moduleSubpath strips "Modules/<Name>/" prefix and returns the remainder, or "" if not a module file.
func moduleSubpath(rel string) string {
	if !strings.HasPrefix(rel, "Modules/") {
		return ""
	}
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

func classify(rel string) (int, string, bool) {
	// Modules/<Name>/<subpath>.php → classify on subpath
	if sub := moduleSubpath(rel); sub != "" {
		if p, k, ok := classifyCore(sub); ok {
			return p, k, true
		}
		return 0, "", false
	}
	return classifyCore(rel)
}

func classifyCore(rel string) (int, string, bool) {
	switch {
	// Standard Laravel
	case strings.HasPrefix(rel, "config/"), strings.HasPrefix(rel, "Config/"):
		return 1, "config", true
	case strings.HasPrefix(rel, "routes/"), strings.HasPrefix(rel, "Routes/"):
		return 1, "route", true
	case strings.HasPrefix(rel, "database/migrations/"), strings.HasPrefix(rel, "Database/Migrations/"):
		return 1, "migration", true
	case strings.HasPrefix(rel, "app/Models/"), strings.HasPrefix(rel, "Models/"),
		strings.HasPrefix(rel, "Entities/"), // nwidart/laravel-modules older convention
		strings.HasPrefix(rel, "app/Repositories/"), strings.HasPrefix(rel, "Repositories/"):
		return 2, "model", true
	case strings.HasPrefix(rel, "app/Http/Controllers/"), strings.HasPrefix(rel, "Http/Controllers/"):
		return 3, "controller", true
	case strings.HasPrefix(rel, "app/Services/"), strings.HasPrefix(rel, "Services/"):
		return 3, "service", true
	case strings.HasPrefix(rel, "app/Http/Requests/"), strings.HasPrefix(rel, "Http/Requests/"):
		return 3, "request", true
	case (strings.HasPrefix(rel, "resources/views/") || strings.HasPrefix(rel, "Resources/views/")) &&
		strings.HasSuffix(rel, ".blade.php"):
		return 4, "blade", true
	}
	return 0, "", false
}
