package env

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aar10n/makepkg/pkg/config"
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

func UpdateEnvForToolchain(env Env, toolchain *config.Toolchain) error {
	env.Set("FILE_DIR", filepath.Dir(toolchain.FilePath))
	var err error
	binPath := toolchain.Bin
	if binPath != "" {
		binPath, err = filepath.Abs(env.Subst(binPath))
		if err != nil {
			return fmt.Errorf("failed to resolve absolute path for toolchain bin: %w", err)
		}
	}

	crossPrefixPath := filepath.Join(binPath, env.Subst(toolchain.CrossPrefix))
	if toolchain.CrossPrefix != "" {
		env.Set("CROSS_PREFIX", env.Subst(toolchain.CrossPrefix))
	}

	for _, prog := range crossPrefixPrograms {
		env.Set(toolToEnvVar(prog), crossPrefixPath+prog)
	}

	for alias, target := range programAliases {
		if targetPath, exists := env.Get(toolToEnvVar(target)); exists {
			env.Set(toolToEnvVar(alias), targetPath)
		}
	}

	for _, prog := range toolchain.ExtraPrograms {
		env.Set(toolToEnvVar(prog), filepath.Join(binPath, prog))
	}
	return nil
}

func toolToEnvVar(name string) string {
	envVar := strings.ToUpper(name)
	envVar = strings.ReplaceAll(envVar, "-", "_")
	envVar = strings.ReplaceAll(envVar, "+", "X")
	return envVar
}
