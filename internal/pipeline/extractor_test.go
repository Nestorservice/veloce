package pipeline

import "testing"

func TestExtractGoTypes_StructWithFields(t *testing.T) {
	src := `package domain

import "time"

type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}
`
	got := ExtractGoTypes(src)
	if len(got) != 1 || got[0].Name != "User" || got[0].Package != "domain" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Fields) != 3 {
		t.Errorf("fields = %v", got[0].Fields)
	}
}

func TestExtractDartTypes_ClassWithFields(t *testing.T) {
	src := `class UserModel {
  final String id;
  final String email;
  final DateTime createdAt;
}`
	got := ExtractDartTypes(src)
	if len(got) != 1 || got[0].Name != "UserModel" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Fields) != 3 {
		t.Errorf("fields = %v", got[0].Fields)
	}
}
