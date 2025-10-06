package build

import (
	"fmt"

	"github.com/aar10n/makepkg/pkg/config"
)

// GetBuildOrder resolves the dependency graph and returns packages in build order.
// Returns an error if there are circular dependencies or missing dependencies.
func GetBuildOrder(cfg *config.Config) ([][]string, error) {
	pkgMap := make(map[string]*config.Package)
	for i := range cfg.Packages {
		pkgMap[cfg.Packages[i].Name] = &cfg.Packages[i]
	}

	for _, pkg := range cfg.Packages {
		for _, dep := range pkg.DependsOn {
			if _, exists := pkgMap[dep]; !exists {
				return nil, fmt.Errorf("package %s depends on non-existent package %s", pkg.Name, dep)
			}
		}
	}

	reverseGraph := make(map[string][]string)
	reverseInDegree := make(map[string]int)
	for _, pkg := range cfg.Packages {
		reverseInDegree[pkg.Name] = len(pkg.DependsOn)
		for _, dep := range pkg.DependsOn {
			reverseGraph[dep] = append(reverseGraph[dep], pkg.Name)
		}
	}

	var result [][]string
	queue := []string{}

	for name, degree := range reverseInDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	processed := 0
	for len(queue) > 0 {
		level := make([]string, len(queue))
		copy(level, queue)
		result = append(result, level)

		newQueue := []string{}
		for _, name := range queue {
			processed++
			for _, dependent := range reverseGraph[name] {
				reverseInDegree[dependent]--
				if reverseInDegree[dependent] == 0 {
					newQueue = append(newQueue, dependent)
				}
			}
		}
		queue = newQueue
	}

	if processed != len(cfg.Packages) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return result, nil
}
