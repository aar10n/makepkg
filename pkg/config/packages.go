package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/aar10n/makepkg/pkg/env"
	"github.com/aar10n/makepkg/pkg/logger"
)

// Package represents a single package definition.
type Package struct {
	Name         string   `yaml:"name" toml:"name"`
	URL          string   `yaml:"url" toml:"url"`
	Build        string   `yaml:"build" toml:"build"`
	Install      string   `yaml:"install" toml:"install"`
	Clean        string   `yaml:"clean,omitempty" toml:"clean,omitempty"`
	Env          []string `yaml:"env,omitempty" toml:"env,omitempty"`
	DependsOn    []string `yaml:"depends_on,omitempty" toml:"depends_on,omitempty"`
	PackagesFile string   `yaml:"-" toml:"-"`
}

func (p *Package) Subst(env env.Env) {
	env = env.Clone()
	env.Set("PKG_NAME", p.Name)
	env.Set("PKG_URL", p.URL)
	env.Set("FILE_DIR", filepath.Dir(p.PackagesFile))

	p.URL = env.Subst(p.URL)
	p.Build = env.Subst(p.Build)
	p.Install = env.Subst(p.Install)
	p.Clean = env.Subst(p.Clean)

	for i, e := range p.Env {
		p.Env[i] = env.Subst(e)
	}
}

// Config represents the overall package configuration file.
type Config struct {
	FilePath  string
	Toolchain Toolchain `yaml:"toolchain" toml:"toolchain"`
	Packages  []Package `yaml:"packages" toml:"packages"`
}

// GetPackageByName finds a package by name in the config.
func (c *Config) GetPackageByName(name string) *Package {
	for i := range c.Packages {
		if c.Packages[i].Name == name {
			return &c.Packages[i]
		}
	}
	return nil
}

// Validate performs comprehensive validation on the configuration.
func (c *Config) Validate() error {
	if len(c.Packages) == 0 {
		return fmt.Errorf("no packages defined")
	}

	pkgNames := make(map[string]bool)
	for i, pkg := range c.Packages {
		if pkg.Name == "" {
			return fmt.Errorf("package at index %d missing name", i)
		}

		if pkgNames[pkg.Name] {
			return fmt.Errorf("duplicate package name: %s", pkg.Name)
		}
		pkgNames[pkg.Name] = true

		if pkg.URL == "" {
			return fmt.Errorf("package %s missing URL", pkg.Name)
		}

		if pkg.Build == "" {
			return fmt.Errorf("package %s missing build command", pkg.Name)
		}

		if pkg.Install == "" {
			return fmt.Errorf("package %s missing install command", pkg.Name)
		}

		for _, dep := range pkg.DependsOn {
			if dep == pkg.Name {
				return fmt.Errorf("package %s depends on itself", pkg.Name)
			}
		}
	}

	if err := c.validateDependencies(); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateDependencies() error {
	pkgMap := make(map[string]*Package)
	for i := range c.Packages {
		pkgMap[c.Packages[i].Name] = &c.Packages[i]
	}

	for _, pkg := range c.Packages {
		for _, dep := range pkg.DependsOn {
			if _, exists := pkgMap[dep]; !exists {
				return fmt.Errorf("package %s depends on non-existent package %s", pkg.Name, dep)
			}
		}
	}

	if err := c.detectCircularDependencies(); err != nil {
		return err
	}

	return nil
}

func (c *Config) detectCircularDependencies() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var visit func(pkgName string) error
	visit = func(pkgName string) error {
		visited[pkgName] = true
		recStack[pkgName] = true

		pkg := c.GetPackageByName(pkgName)
		if pkg == nil {
			return nil
		}

		for _, dep := range pkg.DependsOn {
			if !visited[dep] {
				if err := visit(dep); err != nil {
					return err
				}
			} else if recStack[dep] {
				return fmt.Errorf("circular dependency detected: %s -> %s", pkgName, dep)
			}
		}

		recStack[pkgName] = false
		return nil
	}

	for _, pkg := range c.Packages {
		if !visited[pkg.Name] {
			if err := visit(pkg.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// LoadConfig reads and parses a package configuration file (YAML or TOML).
// If path is empty, it tries to find a config file automatically.
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		logger.Debug("No config file specified, attempting auto-discovery")
		var err error
		configPath, err = findConfigFile()
		if err != nil {
			return nil, fmt.Errorf("no config file found (auto-discovery failed): %w", err)
		}
	}

	logger.Debug("Loading configuration from: %s", configPath)

	var err error
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve packages path: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	switch filepath.Ext(configPath) {
	case ".toml":
		if err := toml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse TOML: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config type: %s", filepath.Ext(configPath))
	}

	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config file path: %w", err)
	}

	config.FilePath = configPath
	for i := range config.Packages {
		config.Packages[i].PackagesFile = configPath
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

func findConfigFile() (string, error) {
	candidates := []string{"packages.yaml", "packages.yml", "packages.toml"}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no config file found (tried: %s)", strings.Join(candidates, ", "))
}
