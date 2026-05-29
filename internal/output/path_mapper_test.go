package output

import "testing"

func TestMapGoPath(t *testing.T) {
	cases := []struct{ src, want string }{
		{"app/Models/User.php", "backend/internal/domain/user.go"},
		{"app/Http/Controllers/AuthController.php", "backend/internal/handler/auth_handler.go"},
		{"app/Services/AuthService.php", "backend/internal/service/auth_service.go"},
		{"config/database.php", "backend/internal/config/database.go"},
		{"routes/api.php", "backend/cmd/api/routes.go"},
	}
	for _, c := range cases {
		got := MapGoPath(c.src)
		if got != c.want {
			t.Errorf("MapGoPath(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestMapDartPath(t *testing.T) {
	cases := []struct{ src, want string }{
		{"resources/views/auth/login.blade.php", "frontend/lib/features/auth/presentation/screens/login_screen.dart"},
		{"resources/views/products/index.blade.php", "frontend/lib/features/products/presentation/screens/index_screen.dart"},
	}
	for _, c := range cases {
		got := MapDartPath(c.src)
		if got != c.want {
			t.Errorf("MapDartPath(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}
