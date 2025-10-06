package pkg

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

var crossPrefixPrograms = []string{
	"ar", "as", "ld", "nm", "objcopy", "objdump", "ranlib", "strip",
	"addr2line", "c++filt", "dlltool", "elfedit", "gprof", "readelf",
	"size", "strings", "gcc", "g++",
}

var programAliases = map[string]string{
	"cc":  "gcc",
	"c++": "g++",
}

// ToolchainTool represents a tool in the toolchain
type ToolchainTool struct {
	Name string
	Path string
}

func (t *ToolchainTool) EnvVar() string {
	env := strings.ToUpper(t.Name)
	env = strings.ReplaceAll(env, "-", "_")
	env = strings.ReplaceAll(env, "+", "X")
	return env
}

type EnvVar struct {
	Key   string
	Value string
}

// Toolchain represents the toolchain configuration
type Toolchain struct {
	Arch        string
	Host        string
	Bin         string
	CrossPrefix string
	PkgsRoot    string
	Tools       map[string]ToolchainTool
}

func NewToolchainFromConfig(cfg *Config, envSubst *EnvSubst) (*Toolchain, error) {
	var toolArch string
	var toolHost string
	var toolBin string
	var crossPrefix string
	var undefined []string
	var allUndefinedVars []string
	if cfg.Toolchain.Arch != "" {
		toolArch, undefined = envSubst.SubstWarnUndefined(cfg.Toolchain.Arch)
		allUndefinedVars = append(allUndefinedVars, undefined...)
		envSubst.AddVar("PKGS_ARCH", toolArch)
	}
	if cfg.Toolchain.Host != "" {
		toolHost, undefined = envSubst.SubstWarnUndefined(cfg.Toolchain.Host)
		allUndefinedVars = append(allUndefinedVars, undefined...)
		envSubst.AddVar("PKGS_HOST", toolHost)
	}
	if cfg.Toolchain.Bin != "" {
		toolBin, undefined = envSubst.SubstWarnUndefined(cfg.Toolchain.Bin)
		allUndefinedVars = append(allUndefinedVars, undefined...)
	}
	if cfg.Toolchain.CrossPrefix != "" {
		crossPrefix, undefined = envSubst.SubstWarnUndefined(cfg.Toolchain.CrossPrefix)
		allUndefinedVars = append(allUndefinedVars, undefined...)
	}

	if toolHost != "" && toolArch == "" {
		return nil, fmt.Errorf("toolchain 'host' is set but 'arch' is not; please set 'arch' as well")
	}

	if toolArch == "" {
		toolArch = runtime.GOARCH
	}
	if toolHost == "" {
		toolHost = fmt.Sprintf("%s-%s", toolArch, runtime.GOOS)
	}

	if len(allUndefinedVars) > 0 {
		// Remove duplicates
		seen := make(map[string]bool)
		unique := make([]string, 0)
		hasPkgsHost := false
		for _, v := range allUndefinedVars {
			if !seen[v] {
				unique = append(unique, v)
				seen[v] = true
				if v == "PKGS_HOST" {
					hasPkgsHost = true
				}
			}
		}

		errMsg := fmt.Sprintf("toolchain configuration references undefined variables: %v", unique)
		if hasPkgsHost {
			errMsg += " (hint: use --host flag or set 'host' in toolchain config to set PKGS_HOST)"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var err error
	if toolBin, err = filepath.Abs(toolBin); err != nil {
		toolBin = envSubst.Subst(cfg.Toolchain.Bin)
	}

	// register cross-prefix toolchain programs
	tools := map[string]ToolchainTool{}
	for _, prog := range crossPrefixPrograms {
		toolPath := filepath.Join(toolBin, crossPrefix+prog)
		tools[prog] = ToolchainTool{prog, toolPath}
	}

	// register tool aliases
	for alias, target := range programAliases {
		if tool, exists := tools[target]; exists {
			tools[alias] = ToolchainTool{alias, tool.Path}
		}
	}

	// register extra programs
	for _, prog := range cfg.Toolchain.ExtraPrograms {
		toolPath := filepath.Join(toolBin, prog)
		tools[prog] = ToolchainTool{prog, toolPath}
	}

	return &Toolchain{
		Arch:        toolArch,
		Host:        toolHost,
		Bin:         toolBin,
		CrossPrefix: crossPrefix,
		PkgsRoot:    cfg.PackagesRoot,
		Tools:       tools,
	}, nil
}

func (t *Toolchain) ToEnvVars() []EnvVar {
	env := []EnvVar{
		{"PKGS_ARCH", t.Arch},
		{"PKGS_HOST", t.Host},
		{"PKGS_ROOT", t.PkgsRoot},
	}

	if t.CrossPrefix != "" {
		env = append(env, EnvVar{Key: "CROSS_PREFIX", Value: t.CrossPrefix})
	}

	for _, tool := range t.Tools {
		env = append(env, EnvVar{Key: tool.EnvVar(), Value: tool.Path})
	}
	return env
}

func (t *Toolchain) String() string {
	if len(t.Tools) == 0 {
		return "No toolchain configured"
	}
	var sb strings.Builder
	sb.WriteString("Toolchain Configuration:\n")
	sb.WriteString("  Architecture: ")
	sb.WriteString(t.Arch)
	sb.WriteString("\n")
	if t.Host != "" {
		sb.WriteString("  Host: ")
		sb.WriteString(t.Host)
		sb.WriteString("\n")
	}
	sb.WriteString("  Package Root: ")
	sb.WriteString(t.PkgsRoot)
	sb.WriteString("\n")
	sb.WriteString("  Toolchain:\n")
	sb.WriteString("    Bin: ")
	sb.WriteString(t.Bin)
	sb.WriteString("\n")
	sb.WriteString("    Cross Prefix: ")
	sb.WriteString(t.CrossPrefix)
	sb.WriteString("\n")
	sb.WriteString("    Tools:\n")
	for _, tool := range t.Tools {
		sb.WriteString("      ")
		sb.WriteString(tool.EnvVar())
		sb.WriteString(": ")
		sb.WriteString(tool.Path)
		sb.WriteString("\n")
	}
	return sb.String()
}
