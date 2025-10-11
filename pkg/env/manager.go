package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aar10n/makepkg/pkg/logger"
)

var reSubst = regexp.MustCompile(`\$\{([^}]+)}`)

type Env interface {
	Get(key string) (string, bool)
	Set(key, value string)
	PrependToVar(key, value, sep string)
	Subst(s string) string
	SubstWarnUndefined(s string) (string, []string)
	AddToEnv(other Env)
	EnvironmentForPackage(pkgName string, pkgEnv []string, sysroot string, makeJobs int) Env
	ToSlice() []string
	Clone() Env
}

// Manager manages environment variables for package builds.
type Manager struct {
	baseEnv map[string]string
}

// NewManager creates a new environment manager.
func NewManager() *Manager {
	env := &Manager{
		baseEnv: make(map[string]string),
	}

	path := os.Getenv("PATH")
	env.baseEnv["PATH"] = path
	return env
}

func (e *Manager) Set(key, value string) {
	//value = e.Subst(value)
	logger.Debug("Setting %s=%s", key, value)
	e.baseEnv[key] = value
}

func (e *Manager) Get(key string) (string, bool) {
	val, ok := e.baseEnv[key]
	return val, ok
}

func (e *Manager) PrependToVar(key, value, sep string) {
	logger.Debug("Prepending to flags %s: %s", key, value)
	if existing, ok := e.baseEnv[key]; ok && strings.TrimSpace(existing) != "" {
		e.baseEnv[key] = value + sep + existing
	} else {
		e.baseEnv[key] = value
	}
}

func (e *Manager) Subst(s string) string {
	return reSubst.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := e.baseEnv[varName]; ok {
			return val
		}
		return match
	})
}

func (e *Manager) SubstWarnUndefined(s string) (string, []string) {
	undefined := make([]string, 0)
	result := reSubst.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := e.baseEnv[varName]; ok {
			return val
		}
		undefined = append(undefined, varName)
		return match
	})
	return result, undefined
}

func (m *Manager) AddToEnv(other Env) {
	if other == nil {
		return
	}
	for key, value := range m.baseEnv {
		other.Set(key, value)
	}
}

// EnvironmentForPackage prepares environment variables for building and installing a package.
func (e *Manager) EnvironmentForPackage(pkgName string, pkgEnv []string, sysroot string, makeJobs int) Env {
	env := e.Clone()
	env.Set("PKG_NAME", pkgName)

	if makeJobs > 0 {
		env.Set("MAKEFLAGS", fmt.Sprintf("-j%d", makeJobs))
	}

	if sysroot != "" {
		pkgConfigPath := filepath.Join(sysroot, "usr", "lib", "pkgconfig")
		env.PrependToVar("PKG_CONFIG_PATH", pkgConfigPath, ":")
		env.Set("PKG_CONFIG_SYSROOT_DIR", sysroot)

		cflags := fmt.Sprintf("-I%s/usr/include", sysroot)
		env.PrependToVar("CFLAGS", cflags, " ")

		cxxflags := fmt.Sprintf("-I%s/usr/include", sysroot)
		env.PrependToVar("CXXFLAGS", cxxflags, " ")

		ldflags := fmt.Sprintf("-L%s/usr/lib -L%s/lib", sysroot, sysroot)
		env.PrependToVar("LDFLAGS", ldflags, " ")

		env.PrependToVar("LIBRARY_PATH", filepath.Join(sysroot, "usr", "lib"), ":")
		env.PrependToVar("LIBRARY_PATH", filepath.Join(sysroot, "lib"), ":")
		env.PrependToVar("LD_LIBRARY_PATH", filepath.Join(sysroot, "usr", "lib"), ":")
		env.PrependToVar("LD_LIBRARY_PATH", filepath.Join(sysroot, "lib"), ":")
	}

	for _, envVar := range pkgEnv {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := env.Subst(parts[1])
			env.Set(key, value)
		} else {
			logger.Debug("Warning: invalid env var format (expected NAME=VALUE): %s", envVar)
		}
	}
	return env
}

func (e *Manager) ToSlice() []string {
	result := make([]string, 0, len(e.baseEnv))
	for k, v := range e.baseEnv {
		result = append(result, k+"="+v)
	}
	return result
}

func (e *Manager) Clone() Env {
	clone := &Manager{
		baseEnv: make(map[string]string, len(e.baseEnv)),
	}
	for k, v := range e.baseEnv {
		clone.baseEnv[k] = v
	}
	return clone
}
