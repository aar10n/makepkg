package build

import (
	"testing"

	"github.com/aar10n/makepkg/pkg/config"
)

func TestBuildOrder_Simple(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "a", URL: "http://a", Build: "make", Install: "make install"},
			{Name: "b", URL: "http://b", Build: "make", Install: "make install", DependsOn: []string{"a"}},
			{Name: "c", URL: "http://c", Build: "make", Install: "make install", DependsOn: []string{"b"}},
		},
	}

	order, err := GetBuildOrder(cfg)
	if err != nil {
		t.Fatalf("GetBuildOrder failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 levels, got %d", len(order))
	}

	if order[0][0] != "a" {
		t.Errorf("Level 0 should be 'a', got %v", order[0])
	}
	if order[1][0] != "b" {
		t.Errorf("Level 1 should be 'b', got %v", order[1])
	}
	if order[2][0] != "c" {
		t.Errorf("Level 2 should be 'c', got %v", order[2])
	}
}

func TestBuildOrder_Parallel(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "a", URL: "http://a", Build: "make", Install: "make install"},
			{Name: "b", URL: "http://b", Build: "make", Install: "make install"},
			{Name: "c", URL: "http://c", Build: "make", Install: "make install"},
		},
	}

	order, err := GetBuildOrder(cfg)
	if err != nil {
		t.Fatalf("GetBuildOrder failed: %v", err)
	}

	if len(order) != 1 {
		t.Fatalf("Expected 1 level, got %d", len(order))
	}

	if len(order[0]) != 3 {
		t.Fatalf("Expected 3 packages in level 0, got %d", len(order[0]))
	}
}

func TestBuildOrder_MultipleDependencies(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "a", URL: "http://a", Build: "make", Install: "make install"},
			{Name: "b", URL: "http://b", Build: "make", Install: "make install"},
			{Name: "c", URL: "http://c", Build: "make", Install: "make install", DependsOn: []string{"a", "b"}},
		},
	}

	order, err := GetBuildOrder(cfg)
	if err != nil {
		t.Fatalf("GetBuildOrder failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("Expected 2 levels, got %d", len(order))
	}

	level0Set := make(map[string]bool)
	for _, name := range order[0] {
		level0Set[name] = true
	}
	if !level0Set["a"] || !level0Set["b"] {
		t.Errorf("Level 0 should contain 'a' and 'b', got %v", order[0])
	}

	if order[1][0] != "c" {
		t.Errorf("Level 1 should be 'c', got %v", order[1])
	}
}

func TestBuildOrder_DiamondDependency(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "a", URL: "http://a", Build: "make", Install: "make install"},
			{Name: "b", URL: "http://b", Build: "make", Install: "make install", DependsOn: []string{"a"}},
			{Name: "c", URL: "http://c", Build: "make", Install: "make install", DependsOn: []string{"a"}},
			{Name: "d", URL: "http://d", Build: "make", Install: "make install", DependsOn: []string{"b", "c"}},
		},
	}

	order, err := GetBuildOrder(cfg)
	if err != nil {
		t.Fatalf("GetBuildOrder failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 levels, got %d", len(order))
	}

	if order[0][0] != "a" {
		t.Errorf("Level 0 should be 'a', got %v", order[0])
	}

	level1Set := make(map[string]bool)
	for _, name := range order[1] {
		level1Set[name] = true
	}
	if !level1Set["b"] || !level1Set["c"] {
		t.Errorf("Level 1 should contain 'b' and 'c', got %v", order[1])
	}

	if order[2][0] != "d" {
		t.Errorf("Level 2 should be 'd', got %v", order[2])
	}
}

func TestBuildOrder_CircularDependency(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "a", URL: "http://a", Build: "make", Install: "make install", DependsOn: []string{"b"}},
			{Name: "b", URL: "http://b", Build: "make", Install: "make install", DependsOn: []string{"a"}},
		},
	}

	_, err := GetBuildOrder(cfg)
	if err == nil {
		t.Fatal("Expected error for circular dependency, got nil")
	}
}

func TestBuildOrder_MissingDependency(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "a", URL: "http://a", Build: "make", Install: "make install", DependsOn: []string{"nonexistent"}},
		},
	}

	_, err := GetBuildOrder(cfg)
	if err == nil {
		t.Fatal("Expected error for missing dependency, got nil")
	}
}

func TestBuildOrder_ComplexGraph(t *testing.T) {
	cfg := &config.Config{
		Packages: []config.Package{
			{Name: "base", URL: "http://base", Build: "make", Install: "make install"},
			{Name: "lib1", URL: "http://lib1", Build: "make", Install: "make install", DependsOn: []string{"base"}},
			{Name: "lib2", URL: "http://lib2", Build: "make", Install: "make install", DependsOn: []string{"base"}},
			{Name: "app1", URL: "http://app1", Build: "make", Install: "make install", DependsOn: []string{"lib1"}},
			{Name: "app2", URL: "http://app2", Build: "make", Install: "make install", DependsOn: []string{"lib1", "lib2"}},
		},
	}

	order, err := GetBuildOrder(cfg)
	if err != nil {
		t.Fatalf("GetBuildOrder failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("Expected 3 levels, got %d", len(order))
	}

	if order[0][0] != "base" {
		t.Errorf("Level 0 should be 'base', got %v", order[0])
	}
}
