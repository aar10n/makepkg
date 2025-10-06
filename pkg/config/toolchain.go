package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/aar10n/makepkg/pkg/logger"
)

// Toolchain represents toolchain configuration.
type Toolchain struct {
	FilePath      string
	Arch          string   `yaml:"arch" toml:"arch"`
	Bin           string   `yaml:"bin" toml:"bin"`
	Host          string   `yaml:"host" toml:"host"`
	CrossPrefix   string   `yaml:"cross_prefix" toml:"cross_prefix"`
	ExtraPrograms []string `yaml:"extra_programs" toml:"extra_programs"`
}

// LoadToolchainConfig reads and parses a standalone toolchain configuration file (YAML or TOML).
// If path is empty, it tries to find a toolchain file automatically.
// Returns the config, the resolved path, and any error.
func LoadToolchainConfig(path string) (*Toolchain, string, error) {
	if path == "" {
		logger.Debug("No toolchain file specified, attempting auto-discovery")
		var err error
		path, err = findToolchainFile()
		if err != nil {
			logger.Debug("No toolchain file found (auto-discovery failed)")
			return nil, "", nil
		}
	}

	logger.Debug("Loading toolchain configuration from: %s", path)

	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve toolchain file path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read toolchain file: %w", err)
	}

	var toolchainConfig Toolchain

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		logger.Debug("Parsing toolchain file as TOML (based on .toml extension)")
		if err := toml.Unmarshal(data, &toolchainConfig); err != nil {
			return nil, "", fmt.Errorf("failed to parse toolchain TOML: %w", err)
		}
	case ".yaml", ".yml":
		logger.Debug("Parsing toolchain file as YAML (based on %s extension)", ext)
		if err := yaml.Unmarshal(data, &toolchainConfig); err != nil {
			return nil, "", fmt.Errorf("failed to parse toolchain YAML: %w", err)
		}
	default:
		logger.Debug("No recognized extension, trying YAML then TOML")
		if err := yaml.Unmarshal(data, &toolchainConfig); err != nil {
			if tomlErr := toml.Unmarshal(data, &toolchainConfig); tomlErr != nil {
				return nil, "", fmt.Errorf("failed to parse toolchain file as YAML or TOML (yaml: %v, toml: %v)", err, tomlErr)
			}
			logger.Debug("Successfully parsed toolchain file as TOML")
		} else {
			logger.Debug("Successfully parsed toolchain file as YAML")
		}
	}

	toolchainConfig.FilePath = path
	logger.Debug("Successfully loaded toolchain configuration from %s", path)
	return &toolchainConfig, path, nil
}

// MergeToolchainConfig merges a higher priority toolchain config into a base config.
// Non-empty fields from the override config take precedence.
func MergeToolchainConfig(base, override *Toolchain) Toolchain {
	logger.Debug("Merging toolchain configurations")
	result := *base

	if override.Arch != "" {
		logger.Debug("  Overriding Arch: %q -> %q", base.Arch, override.Arch)
		result.Arch = override.Arch
	}
	if override.Bin != "" {
		logger.Debug("  Overriding Bin: %q -> %q", base.Bin, override.Bin)
		result.Bin = override.Bin
	}
	if override.Host != "" {
		logger.Debug("  Overriding Host: %q -> %q", base.Host, override.Host)
		result.Host = override.Host
	}
	if override.CrossPrefix != "" {
		logger.Debug("  Overriding CrossPrefix: %q -> %q", base.CrossPrefix, override.CrossPrefix)
		result.CrossPrefix = override.CrossPrefix
	}
	if len(override.ExtraPrograms) > 0 {
		logger.Debug("  Overriding ExtraPrograms (%d programs)", len(override.ExtraPrograms))
		result.ExtraPrograms = override.ExtraPrograms
	}

	return result
}

func findToolchainFile() (string, error) {
	candidates := []string{"toolchain.yaml", "toolchain.yml", "toolchain.toml"}

	logger.Debug("Searching for toolchain file in current directory")
	for _, candidate := range candidates {
		logger.Debug("  Trying: %s", candidate)
		if _, err := os.Stat(candidate); err == nil {
			logger.Debug("  Found toolchain file: %s", candidate)
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no toolchain file found (tried: %s)", strings.Join(candidates, ", "))
}
