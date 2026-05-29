package state

import (
	"strings"
	"testing"
)

func TestSharedTypes_AddAndRender(t *testing.T) {
	st := NewSharedTypes(t.TempDir())
	st.AddGoType(GoType{Name: "User", Package: "domain", File: "user.go", Fields: []string{"ID uuid.UUID", "Email string"}})
	st.AddDartType(DartType{Name: "UserModel", File: "features/auth/domain/user_model.dart", Fields: []string{"String id", "String email"}})

	out := st.RenderForPrompt()
	if !strings.Contains(out, "User") || !strings.Contains(out, "UserModel") {
		t.Errorf("rendered prompt missing type names: %s", out)
	}
}

func TestSharedTypes_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	st := NewSharedTypes(dir)
	st.AddGoType(GoType{Name: "Product", Package: "domain", File: "product.go", Fields: []string{"ID uuid.UUID"}})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSharedTypes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.GetGoType("Product"); !ok {
		t.Errorf("Product type missing after load")
	}
}
