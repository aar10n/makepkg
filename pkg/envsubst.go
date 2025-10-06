package pkg

import (
	"os"
	"regexp"
)

// EnvSubst performs environment variable substitution
type EnvSubst struct {
	vars map[string]string
}

// EnvSubstOption is a functional option for EnvSubst
type EnvSubstOption func(*EnvSubst)

// WithEnviron adds variables from os.Environ()
func WithEnviron() EnvSubstOption {
	return func(e *EnvSubst) {
		for _, env := range os.Environ() {
			for i := 0; i < len(env); i++ {
				if env[i] == '=' {
					e.vars[env[:i]] = env[i+1:]
					break
				}
			}
		}
	}
}

// WithEnv adds variables from environment strings (KEY=VALUE format)
func WithEnv(environ ...string) EnvSubstOption {
	return func(e *EnvSubst) {
		for _, env := range environ {
			for i := 0; i < len(env); i++ {
				if env[i] == '=' {
					e.vars[env[:i]] = env[i+1:]
					break
				}
			}
		}
	}
}

// WithEnvMap adds variables from a map
func WithEnvMap(env map[string]string) EnvSubstOption {
	return func(e *EnvSubst) {
		for k, v := range env {
			e.vars[k] = v
		}
	}
}

// NewEnvSubst creates a new EnvSubst with the given options
func NewEnvSubst(opts ...EnvSubstOption) *EnvSubst {
	e := &EnvSubst{
		vars: make(map[string]string),
	}

	// Apply options
	for _, opt := range opts {
		opt(e)
	}

	return e
}

// AddVar adds or updates a variable
func (e *EnvSubst) AddVar(name, value string) {
	e.vars[name] = value
}

// ToEnv returns the variables as a slice of "KEY=VALUE" strings
func (e *EnvSubst) ToEnv() []string {
	env := make([]string, 0, len(e.vars))
	for k, v := range e.vars {
		env = append(env, k+"="+v)
	}
	return env
}

// Subst substitutes ${VAR} occurrences with corresponding values
func (e *EnvSubst) Subst(s string) string {
	re := regexp.MustCompile(`\$\{([^}]+)}`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name (strip ${ and })
		varName := match[2 : len(match)-1]
		if val, ok := e.vars[varName]; ok {
			return val
		}
		return match
	})
}

// SubstWarnUndefined substitutes variables and returns a warning if any are undefined
func (e *EnvSubst) SubstWarnUndefined(s string) (string, []string) {
	re := regexp.MustCompile(`\$\{([^}]+)}`)
	undefined := make([]string, 0)
	result := re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := e.vars[varName]; ok {
			return val
		}
		undefined = append(undefined, varName)
		return match
	})
	return result, undefined
}
