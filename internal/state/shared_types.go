package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type GoType struct {
	Name    string   `json:"-"`
	Package string   `json:"package"`
	File    string   `json:"file"`
	Fields  []string `json:"fields"`
}

type DartType struct {
	Name   string   `json:"-"`
	File   string   `json:"file"`
	Fields []string `json:"fields"`
}

type sharedTypesData struct {
	GoTypes   map[string]GoType   `json:"go_types"`
	DartTypes map[string]DartType `json:"dart_types"`
}

type SharedTypes struct {
	mu      sync.RWMutex
	rootDir string
	data    sharedTypesData
}

func NewSharedTypes(outputDir string) *SharedTypes {
	return &SharedTypes{
		rootDir: outputDir,
		data: sharedTypesData{
			GoTypes:   map[string]GoType{},
			DartTypes: map[string]DartType{},
		},
	}
}

func LoadSharedTypes(outputDir string) (*SharedTypes, error) {
	path := filepath.Join(outputDir, ".veloce", "shared_types.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewSharedTypes(outputDir), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read shared_types: %w", err)
	}
	var d sharedTypesData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("parse shared_types: %w", err)
	}
	if d.GoTypes == nil {
		d.GoTypes = map[string]GoType{}
	}
	if d.DartTypes == nil {
		d.DartTypes = map[string]DartType{}
	}
	return &SharedTypes{rootDir: outputDir, data: d}, nil
}

func (s *SharedTypes) AddGoType(t GoType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.GoTypes[t.Name] = t
}

func (s *SharedTypes) AddDartType(t DartType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.DartTypes[t.Name] = t
}

func (s *SharedTypes) GetGoType(name string) (GoType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data.GoTypes[name]
	return t, ok
}

func (s *SharedTypes) RenderForPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	b.WriteString("# Existing Go types\n")
	for name, t := range s.data.GoTypes {
		fmt.Fprintf(&b, "type %s struct { %s } // pkg %s, %s\n",
			name, strings.Join(t.Fields, "; "), t.Package, t.File)
	}
	b.WriteString("\n# Existing Dart types\n")
	for name, t := range s.data.DartTypes {
		fmt.Fprintf(&b, "class %s { %s } // %s\n",
			name, strings.Join(t.Fields, "; "), t.File)
	}
	return b.String()
}

func (s *SharedTypes) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir := filepath.Join(s.rootDir, ".veloce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "shared_types.json.tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()
	return os.Rename(tmpPath, filepath.Join(dir, "shared_types.json"))
}
