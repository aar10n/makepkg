package pkg

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var VerboseLogging = false

func debugLog(format string, args ...interface{}) {
	if VerboseLogging {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// BuildEnvironment prepares environment variables for building packages
func BuildEnvironment(pkg *Package, sysroot string, toolchain *Toolchain, envSubst *EnvSubst) []string {
	debugLog("Preparing build environment for package: %s", pkg.Name)
	env := os.Environ()

	// Add PKGS_HOST if specified
	if toolchain.Host != "" {
		debugLog("Setting PKGS_HOST: %s", toolchain.Host)
		env = appendOrReplace(env, "PKGS_HOST", toolchain.Host)
	}

	// Prepend toolchain bin to PATH so tools are found first
	if toolchain.Bin != "" {
		debugLog("Adding toolchain bin to PATH: %s", toolchain.Bin)
		env = prependToPathVar(env, "PATH", toolchain.Bin)
	}

	// Add the toolchain environment variables
	for _, v := range toolchain.ToEnvVars() {
		debugLog("Setting toolchain env var: %s=%s", v.Key, v.Value)
		env = appendOrReplace(env, v.Key, v.Value)
	}

	// If sysroot is specified, configure paths
	if sysroot != "" {
		absRoot, err := filepath.Abs(sysroot)
		if err == nil {
			sysroot = absRoot
		}
		debugLog("Using sysroot: %s", sysroot)

		env = appendOrReplace(env, "SYS_ROOT", sysroot)

		// Set up PKG_CONFIG paths
		pkgConfigPath := filepath.Join(sysroot, "usr", "lib", "pkgconfig")
		pkgConfigPath += ":" + filepath.Join(sysroot, "usr", "share", "pkgconfig")
		debugLog("Setting PKG_CONFIG_PATH: %s", pkgConfigPath)
		env = appendOrReplace(env, "PKG_CONFIG_PATH", pkgConfigPath)
		env = appendOrReplace(env, "PKG_CONFIG_SYSROOT_DIR", sysroot)

		// Set library and include paths
		cflags := fmt.Sprintf("-I%s/usr/include", sysroot)
		cxxflags := fmt.Sprintf("-I%s/usr/include", sysroot)
		ldflags := fmt.Sprintf("-L%s/usr/lib -L%s/lib", sysroot, sysroot)

		debugLog("Setting CFLAGS: %s", cflags)
		debugLog("Setting CXXFLAGS: %s", cxxflags)
		debugLog("Setting LDFLAGS: %s", ldflags)

		env = prependToPath(env, "CFLAGS", cflags)
		env = prependToPath(env, "CXXFLAGS", cxxflags)
		env = prependToPath(env, "LDFLAGS", ldflags)

		// Set library path for runtime
		env = prependToPath(env, "LIBRARY_PATH", filepath.Join(sysroot, "usr", "lib"))
		env = prependToPath(env, "LIBRARY_PATH", filepath.Join(sysroot, "lib"))
		env = prependToPath(env, "LD_LIBRARY_PATH", filepath.Join(sysroot, "usr", "lib"))
		env = prependToPath(env, "LD_LIBRARY_PATH", filepath.Join(sysroot, "lib"))
	}

	// Add package-specific environment variables
	if len(pkg.Env) > 0 {
		debugLog("Adding package-specific environment variables")
		for _, envVar := range pkg.Env {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				// Expand variables in the value using envSubst
				if envSubst != nil {
					value = envSubst.Subst(value)
				}
				debugLog("Setting package env var: %s=%s", key, value)
				env = appendOrReplace(env, key, value)
			} else {
				debugLog("Warning: invalid env var format (expected NAME=VALUE): %s", envVar)
			}
		}
	}

	return env
}

// InstallEnvironment prepares environment variables for installing packages
func InstallEnvironment(pkg *Package, sysroot string, toolchain *Toolchain, envSubst *EnvSubst) []string {
	env := BuildEnvironment(pkg, sysroot, toolchain, envSubst)

	// Set INSTALL_ROOT
	installRoot := sysroot
	if installRoot == "" {
		installRoot = "/"
	} else {
		absRoot, err := filepath.Abs(installRoot)
		if err == nil {
			installRoot = absRoot
		}
	}
	env = appendOrReplace(env, "INSTALL_ROOT", installRoot)

	return env
}

// appendOrReplace adds or replaces an environment variable
func appendOrReplace(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// prependToPath prepends a value to a path-like environment variable (space-separated)
func prependToPath(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			existing := strings.TrimPrefix(e, prefix)
			if existing != "" {
				env[i] = prefix + value + " " + existing
			} else {
				env[i] = prefix + value
			}
			return env
		}
	}
	return append(env, prefix+value)
}

// prependToPathVar prepends a value to a PATH-style environment variable (colon-separated)
func prependToPathVar(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			existing := strings.TrimPrefix(e, prefix)
			if existing != "" {
				env[i] = prefix + value + ":" + existing
			} else {
				env[i] = prefix + value
			}
			return env
		}
	}
	return append(env, prefix+value)
}
