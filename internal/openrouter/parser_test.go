package openrouter

import (
	"testing"
)

func TestParseBatchResponse_MultipleFiles(t *testing.T) {
	resp := `
=== GO: backend/internal/domain/user.go ===
package domain

type User struct {
	ID    int
	Email string
}

=== DART: frontend/lib/features/auth/presentation/screens/login_screen.dart ===
import 'package:flutter/material.dart';

class LoginScreen extends StatelessWidget {
  const LoginScreen({super.key});
  @override
  Widget build(BuildContext context) => const Placeholder();
}

=== GO: backend/internal/handler/user_handler.go ===
package handler

import "net/http"

func GetUser(w http.ResponseWriter, r *http.Request) {}
`
	files := ParseBatchResponse(resp)
	if len(files) != 3 {
		t.Fatalf("parsed %d files, want 3", len(files))
	}

	// File 0: Go domain
	if files[0].Path != "backend/internal/domain/user.go" {
		t.Errorf("file[0].Path = %q", files[0].Path)
	}
	if files[0].Lang != "go" {
		t.Errorf("file[0].Lang = %q", files[0].Lang)
	}
	if !containsStr(files[0].Content, "type User struct") {
		t.Errorf("file[0].Content missing struct: %q", files[0].Content)
	}

	// File 1: Dart screen
	if files[1].Lang != "dart" {
		t.Errorf("file[1].Lang = %q, want dart", files[1].Lang)
	}
	if !containsStr(files[1].Content, "LoginScreen") {
		t.Errorf("file[1].Content missing LoginScreen")
	}

	// File 2: Go handler
	if files[2].Path != "backend/internal/handler/user_handler.go" {
		t.Errorf("file[2].Path = %q", files[2].Path)
	}
}

func TestParseBatchResponse_PreambleDiscarded(t *testing.T) {
	resp := `Here is the translation:

Some explanation text that should be discarded.

=== GO: backend/internal/domain/product.go ===
package domain

type Product struct{ ID int }
`
	files := ParseBatchResponse(resp)
	if len(files) != 1 {
		t.Fatalf("parsed %d files, want 1", len(files))
	}
	if !containsStr(files[0].Content, "type Product struct") {
		t.Errorf("content = %q", files[0].Content)
	}
}

func TestParseBatchResponse_EmptyResponse(t *testing.T) {
	files := ParseBatchResponse("")
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestParseBatchResponse_NoDelimiters(t *testing.T) {
	files := ParseBatchResponse("sorry I cannot do this")
	if len(files) != 0 {
		t.Errorf("got %d files from no-delimiter response", len(files))
	}
}

func TestParseDelimiter(t *testing.T) {
	cases := []struct {
		line    string
		wantOK  bool
		wantLang string
		wantPath string
	}{
		{"=== GO: internal/handler/user_handler.go ===", true, "go", "internal/handler/user_handler.go"},
		{"=== DART: lib/features/auth/login_screen.dart ===", true, "dart", "lib/features/auth/login_screen.dart"},
		{"=== PHP: app/Models/User.php ===", false, "", ""},
		{"just a normal line", false, "", ""},
		{"=== GO:  ===", false, "", ""},    // empty path
		{"  === GO: foo.go ===  ", true, "go", "foo.go"}, // trimmed
	}
	for _, tc := range cases {
		path, lang, ok := parseDelimiter(tc.line)
		if ok != tc.wantOK {
			t.Errorf("parseDelimiter(%q) ok=%v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if ok {
			if lang != tc.wantLang {
				t.Errorf("parseDelimiter(%q) lang=%q, want %q", tc.line, lang, tc.wantLang)
			}
			if path != tc.wantPath {
				t.Errorf("parseDelimiter(%q) path=%q, want %q", tc.line, path, tc.wantPath)
			}
		}
	}
}

func TestValidateParsedFiles(t *testing.T) {
	good := []ParsedFile{
		{Path: "backend/internal/domain/user.go", Lang: "go", Content: "package domain\n\ntype User struct{}\n"},
		{Path: "frontend/lib/features/auth/screens/login.dart", Lang: "dart", Content: "import 'dart:core';\nclass X {}\n"},
	}
	if errs := ValidateParsedFiles(good); len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	bad := []ParsedFile{
		{Path: "", Lang: "go", Content: "package main"},              // empty path
		{Path: "foo.go", Lang: "go", Content: "  "},                  // trivial content
		{Path: "bar.go", Lang: "go", Content: "import \"fmt\"\n"},    // missing package
	}
	errs := ValidateParsedFiles(bad)
	if len(errs) == 0 {
		t.Error("expected validation errors for bad files")
	}
}

func TestSnakeCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ProductController", "product_controller"},
		{"User", "user"},
		{"SmStudentAdmission", "sm_student_admission"},
		{"already_snake", "already_snake"},
		{"ABC", "a_b_c"},
	}
	for _, tc := range cases {
		got := snakeCase(tc.in)
		if got != tc.want {
			t.Errorf("snakeCase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGoOutputPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"app/Models/User.php", "backend/internal/domain/user.go"},
		{"app/Http/Controllers/UserController.php", "backend/internal/handler/user_controller_handler.go"},
		{"app/Http/Requests/StoreUserRequest.php", "backend/internal/handler/store_user_request_request.go"},
		{"config/database.php", "backend/internal/config/database.go"},
		{"routes/api.php", "backend/internal/router/routes.go"},
	}
	for _, tc := range cases {
		got := GoOutputPath(tc.in)
		if got != tc.want {
			t.Errorf("GoOutputPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
