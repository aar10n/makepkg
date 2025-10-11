package env

import (
	"strconv"
	"strings"
)

type mergedEnv struct {
	envs []Env
}

func NewMergedEnv(envs ...Env) Env {
	return &mergedEnv{envs: envs}
}

func (m *mergedEnv) Get(key string) (string, bool) {
	for _, env := range m.envs {
		if val, ok := env.Get(key); ok {
			return val, true
		}
	}
	return "", false
}

func (m *mergedEnv) Set(key, value string) {
	if len(m.envs) == 0 {
		return
	}

	m.envs[0].Set(key, value)
}

func (m *mergedEnv) PrependToVar(key, value, sep string) {
	if len(m.envs) == 0 {
		return
	}

	m.envs[0].PrependToVar(key, value, sep)
}

func (m *mergedEnv) AddToEnv(other Env) {
	if other == nil {
		return
	}
	for i := len(m.envs) - 1; i >= 0; i-- {
		m.envs[i].AddToEnv(other)
	}
}

func (m *mergedEnv) Subst(s string) string {
	return reSubst.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := m.Get(varName); ok {
			return val
		}
		return match
	})
}

func (m *mergedEnv) SubstWarnUndefined(s string) (string, []string) {
	undefined := make([]string, 0)
	result := reSubst.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := m.Get(varName); ok {
			return val
		}
		undefined = append(undefined, varName)
		return match
	})
	return result, undefined
}

func (m *mergedEnv) EnvironmentForPackage(pkgName string, pkgEnv []string, sysroot string, makeJobs int) Env {
	newEnv := NewManager()
	for i := len(m.envs) - 1; i >= 0; i-- {
		m.envs[i].AddToEnv(newEnv)
	}

	for _, kv := range pkgEnv {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			newEnv.Set(parts[0], parts[1])
		}
	}

	newEnv.Set("PKG_NAME", pkgName)
	if sysroot != "" {
		newEnv.Set("SYSROOT", sysroot)
	}
	if makeJobs > 0 {
		newEnv.Set("MAKEJOBS", strconv.Itoa(makeJobs))
	}
	return newEnv
}

func (m *mergedEnv) ToSlice() []string {
	result := make([]string, 0)
	seen := make(map[string]bool)
	for i := len(m.envs) - 1; i >= 0; i-- {
		for _, kv := range m.envs[i].ToSlice() {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				if !seen[key] {
					result = append(result, kv)
					seen[key] = true
				}
			}
		}
	}
	// Reverse the result to maintain the correct order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func (m *mergedEnv) Clone() Env {
	clonedEnvs := make([]Env, len(m.envs))
	for i, env := range m.envs {
		clonedEnvs[i] = env.Clone()
	}
	return &mergedEnv{envs: clonedEnvs}
}
