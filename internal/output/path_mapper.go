package output

import (
	"path/filepath"
	"strings"
	"unicode"
)

// MapGoPath translates a Laravel source path to its Go target path.
func MapGoPath(src string) string {
	src = filepath.ToSlash(src)
	switch {
	case strings.HasPrefix(src, "app/Models/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), ".php"))
		return "backend/internal/domain/" + name + ".go"
	case strings.HasPrefix(src, "app/Http/Controllers/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Controller.php"))
		return "backend/internal/handler/" + name + "_handler.go"
	case strings.HasPrefix(src, "app/Services/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Service.php"))
		return "backend/internal/service/" + name + "_service.go"
	case strings.HasPrefix(src, "app/Repositories/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Repository.php"))
		return "backend/internal/repository/" + name + "_repository.go"
	case strings.HasPrefix(src, "app/Http/Requests/"):
		name := snake(strings.TrimSuffix(filepath.Base(src), "Request.php"))
		return "backend/internal/handler/" + name + "_request.go"
	case strings.HasPrefix(src, "config/"):
		name := strings.TrimSuffix(filepath.Base(src), ".php")
		return "backend/internal/config/" + name + ".go"
	case strings.HasPrefix(src, "routes/"):
		return "backend/cmd/api/routes.go"
	case strings.HasPrefix(src, "database/migrations/"):
		name := strings.TrimSuffix(filepath.Base(src), ".php")
		return "backend/migrations/" + name + ".sql"
	}
	return "backend/_unmapped/" + filepath.Base(src) + ".go"
}

// MapDartPath translates a Blade view to its Flutter screen path.
// e.g. resources/views/auth/login.blade.php → frontend/lib/features/auth/presentation/screens/login_screen.dart
func MapDartPath(src string) string {
	src = filepath.ToSlash(src)
	src = strings.TrimPrefix(src, "resources/views/")
	src = strings.TrimSuffix(src, ".blade.php")
	parts := strings.Split(src, "/")
	if len(parts) < 2 {
		return "frontend/lib/features/_misc/presentation/screens/" + snake(parts[0]) + "_screen.dart"
	}
	feature := parts[0]
	screen := snake(strings.Join(parts[1:], "_"))
	return "frontend/lib/features/" + feature + "/presentation/screens/" + screen + "_screen.dart"
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
