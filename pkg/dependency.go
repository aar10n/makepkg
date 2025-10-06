package pkg

import (
	"fmt"
)

// BuildOrder resolves the dependency graph and returns packages in build order.
// Returns an error if there are circular dependencies or missing dependencies.
func BuildOrder(config *Config) ([][]string, error) {
	pkgMap := make(map[string]*Package)
	for i := range config.Packages {
		pkgMap[config.Packages[i].Name] = &config.Packages[i]
	}

	for _, pkg := range config.Packages {
		for _, dep := range pkg.DependsOn {
			if _, exists := pkgMap[dep]; !exists {
				return nil, fmt.Errorf("package %s depends on non-existent package %s", pkg.Name, dep)
			}
		}
	}

	reverseGraph := make(map[string][]string)
	reverseInDegree := make(map[string]int)
	for _, pkg := range config.Packages {
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

	if processed != len(config.Packages) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return result, nil
}
