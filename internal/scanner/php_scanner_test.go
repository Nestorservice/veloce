package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupLaravelTree(t *testing.T) string {
	root := t.TempDir()
	files := map[string]string{
		"config/database.php":                       "<?php",
		"routes/api.php":                            "<?php",
		"database/migrations/2024_create_users.php": "<?php",
		"app/Models/User.php":                       "<?php",
		"app/Models/Product.php":                    "<?php",
		"app/Http/Controllers/AuthController.php":   "<?php",
		"app/Services/AuthService.php":              "<?php",
		"app/Http/Requests/LoginRequest.php":        "<?php",
		"resources/views/auth/login.blade.php":      "<html>",
		"vendor/symfony/something.php":              "<?php",
		"storage/framework/cache.php":               "<?php",
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScan_ClassifiesIntoPhases(t *testing.T) {
	root := setupLaravelTree(t)
	files, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	counts := map[int]int{}
	for _, f := range files {
		counts[f.Phase]++
	}

	if counts[1] != 3 {
		t.Errorf("phase 1 = %d, want 3 (config + routes + migration)", counts[1])
	}
	if counts[2] != 2 {
		t.Errorf("phase 2 = %d, want 2 (User + Product)", counts[2])
	}
	if counts[3] != 3 {
		t.Errorf("phase 3 = %d, want 3 (Controller + Service + Request)", counts[3])
	}
	if counts[4] != 1 {
		t.Errorf("phase 4 = %d, want 1 (blade)", counts[4])
	}
}

func TestScan_SkipsVendorAndStorage(t *testing.T) {
	root := setupLaravelTree(t)
	files, _ := Scan(root)
	for _, f := range files {
		if strings.HasPrefix(f.RelPath, "vendor/") || strings.HasPrefix(f.RelPath, "storage/") {
			t.Errorf("vendor/storage file leaked: %s", f.RelPath)
		}
	}
}
