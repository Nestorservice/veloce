package output

import (
	"os"
	"path/filepath"
)

// WriteGoFile maps src → target Go path under outputDir and writes content atomically.
func WriteGoFile(outputDir, srcPath, content string) (string, error) {
	rel := MapGoPath(srcPath)
	full := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
