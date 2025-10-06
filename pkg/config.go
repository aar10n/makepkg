package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// Package represents a single package definition
type Package struct {
	Name      string   `yaml:"name" toml:"name"`
	URL       string   `yaml:"url" toml:"url"`
	Build     string   `yaml:"build" toml:"build"`
	Install   string   `yaml:"install" toml:"install"`
	Clean     string   `yaml:"clean,omitempty" toml:"clean,omitempty"`
	Env       []string `yaml:"env,omitempty" toml:"env,omitempty"`
	DependsOn []string `yaml:"depends_on,omitempty" toml:"depends_on,omitempty"`
}

func (p *Package) BuildScript(subst *EnvSubst) string {
	return subst.Subst(p.Build)
}

func (p *Package) InstallScript(subst *EnvSubst) string {
	return subst.Subst(p.Install)
}

func (p *Package) CleanScript(subst *EnvSubst) string {
	return subst.Subst(p.Clean)
}

// ToolchainConfig represents toolchain settings
type ToolchainConfig struct {
	Arch          string   `yaml:"arch" toml:"arch"`
	Bin           string   `yaml:"bin" toml:"bin"`
	Host          string   `yaml:"host" toml:"host"`
	CrossPrefix   string   `yaml:"cross_prefix" toml:"cross_prefix"`
	ExtraPrograms []string `yaml:"extra_programs" toml:"extra_programs"`
}

// Config represents the overall package configuration file
type Config struct {
	ConfigPath   string
	PackagesRoot string
	Toolchain    ToolchainConfig `yaml:"toolchain" toml:"toolchain"`
	Packages     []Package       `yaml:"packages" toml:"packages"`
}

// LoadConfig reads and parses a package configuration file (YAML or TOML).
// If path is empty, it tries to find a config file automatically.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = findConfigFile()
		if err != nil {
			return nil, err
		}
	}

	pkgsRoot, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve packages root: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config

	// Determine format based on file extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		if err := toml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse TOML: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	default:
		// Try YAML first, then TOML
		if err := yaml.Unmarshal(data, &config); err != nil {
			if tomlErr := toml.Unmarshal(data, &config); tomlErr != nil {
				return nil, fmt.Errorf("failed to parse as YAML or TOML (yaml: %v, toml: %v)", err, tomlErr)
			}
		}
	}

	// Validate packages
	if len(config.Packages) == 0 {
		return nil, fmt.Errorf("no packages defined in config")
	}

	for i, pkg := range config.Packages {
		if pkg.Name == "" {
			return nil, fmt.Errorf("package at index %d missing name", i)
		}
		if pkg.URL == "" {
			return nil, fmt.Errorf("package %s missing URL", pkg.Name)
		}
		if pkg.Build == "" {
			return nil, fmt.Errorf("package %s missing build command", pkg.Name)
		}
		if pkg.Install == "" {
			return nil, fmt.Errorf("package %s missing install command", pkg.Name)
		}
	}

	config.PackagesRoot = pkgsRoot
	config.ConfigPath = path
	return &config, nil
}

// findConfigFile searches for a configuration file in the current directory
// Tries packages.yaml, packages.yml, and packages.toml in order
func findConfigFile() (string, error) {
	candidates := []string{"packages.yaml", "packages.yml", "packages.toml"}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no config file found (tried: %s)", strings.Join(candidates, ", "))
}

// LoadToolchainConfig reads and parses a standalone toolchain configuration file (YAML or TOML).
// If path is empty, it tries to find a toolchain file automatically.
func LoadToolchainConfig(path string) (*ToolchainConfig, error) {
	if path == "" {
		debugLog("No toolchain file specified, attempting auto-discovery")
		var err error
		path, err = findToolchainFile()
		if err != nil {
			// No toolchain file found is not an error - it's optional
			debugLog("No toolchain file found (auto-discovery failed)")
			return nil, nil
		}
	}

	debugLog("Loading toolchain configuration from: %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read toolchain file: %w", err)
	}

	var toolchainConfig ToolchainConfig

	// Determine format based on file extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		debugLog("Parsing toolchain file as TOML (based on .toml extension)")
		if err := toml.Unmarshal(data, &toolchainConfig); err != nil {
			return nil, fmt.Errorf("failed to parse toolchain TOML: %w", err)
		}
	case ".yaml", ".yml":
		debugLog("Parsing toolchain file as YAML (based on %s extension)", ext)
		if err := yaml.Unmarshal(data, &toolchainConfig); err != nil {
			return nil, fmt.Errorf("failed to parse toolchain YAML: %w", err)
		}
	default:
		debugLog("No recognized extension, trying YAML then TOML")
		// Try YAML first, then TOML
		if err := yaml.Unmarshal(data, &toolchainConfig); err != nil {
			if tomlErr := toml.Unmarshal(data, &toolchainConfig); tomlErr != nil {
				return nil, fmt.Errorf("failed to parse toolchain file as YAML or TOML (yaml: %v, toml: %v)", err, tomlErr)
			}
			debugLog("Successfully parsed toolchain file as TOML")
		} else {
			debugLog("Successfully parsed toolchain file as YAML")
		}
	}

	debugLog("Successfully loaded toolchain configuration from %s", path)
	return &toolchainConfig, nil
}

// findToolchainFile searches for a toolchain configuration file in the current directory
// Tries toolchain.yaml, toolchain.yml, and toolchain.toml in order
func findToolchainFile() (string, error) {
	candidates := []string{"toolchain.yaml", "toolchain.yml", "toolchain.toml"}

	debugLog("Searching for toolchain file in current directory")
	for _, candidate := range candidates {
		debugLog("  Trying: %s", candidate)
		if _, err := os.Stat(candidate); err == nil {
			debugLog("  Found toolchain file: %s", candidate)
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no toolchain file found (tried: %s)", strings.Join(candidates, ", "))
}

// MergeToolchainConfig merges a higher priority toolchain config into a base config.
// Non-empty fields from the override config take precedence.
func MergeToolchainConfig(base, override *ToolchainConfig) ToolchainConfig {
	debugLog("Merging toolchain configurations")
	result := *base

	if override.Arch != "" {
		debugLog("  Overriding Arch: %q -> %q", base.Arch, override.Arch)
		result.Arch = override.Arch
	}
	if override.Bin != "" {
		debugLog("  Overriding Bin: %q -> %q", base.Bin, override.Bin)
		result.Bin = override.Bin
	}
	if override.Host != "" {
		debugLog("  Overriding Host: %q -> %q", base.Host, override.Host)
		result.Host = override.Host
	}
	if override.CrossPrefix != "" {
		debugLog("  Overriding CrossPrefix: %q -> %q", base.CrossPrefix, override.CrossPrefix)
		result.CrossPrefix = override.CrossPrefix
	}
	if len(override.ExtraPrograms) > 0 {
		debugLog("  Overriding ExtraPrograms (%d programs)", len(override.ExtraPrograms))
		result.ExtraPrograms = override.ExtraPrograms
	}

	return result
}

// GetPackageByName finds a package by name in the config
func (c *Config) GetPackageByName(name string) *Package {
	for i := range c.Packages {
		if c.Packages[i].Name == name {
			return &c.Packages[i]
		}
	}
	return nil
}
