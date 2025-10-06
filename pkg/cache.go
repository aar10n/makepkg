package pkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	cacheFileName  = "makepkg.json"
	sourceDir      = "source"
)

// CacheInfo stores the cached build information for a package
type CacheInfo struct {
	URL     string   `json:"url"`
	Build   string   `json:"build"`
	Install string   `json:"install"`
	Env     []string `json:"env"`
	Host    string   `json:"host"`
	Sysroot string   `json:"sysroot"`
}

// Cache encapsulates all package caching logic
type Cache struct {
	buildDir string
}

// NewCache creates a new Cache instance
func NewCache(buildDir string) *Cache {
	return &Cache{
		buildDir: buildDir,
	}
}

// Read reads the cached build information for a package
func (c *Cache) Read(pkgName string) (*CacheInfo, error) {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	cachePath := filepath.Join(pkgDir, cacheFileName)

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache exists
		}
		return nil, fmt.Errorf("failed to read cache: %w", err)
	}

	var cache CacheInfo
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}

	return &cache, nil
}

// write writes the cache information to the cache file
func (c *Cache) write(pkgName string, cache *CacheInfo) error {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	cachePath := filepath.Join(pkgDir, cacheFileName)

	// Ensure package directory exists
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// WriteBuild writes just the build step cache
func (c *Cache) WriteBuild(pkgName, sysroot, host string, pkg *Package) error {
	// Read existing cache if it exists
	cache, err := c.Read(pkgName)
	if err != nil {
		return fmt.Errorf("failed to read existing cache: %w", err)
	}
	if cache == nil {
		cache = &CacheInfo{}
	}

	// Update build-related fields
	cache.URL = pkg.URL
	cache.Build = pkg.Build
	cache.Env = pkg.Env
	cache.Host = host
	cache.Sysroot = sysroot

	return c.write(pkgName, cache)
}

// WriteInstall writes just the install step cache
func (c *Cache) WriteInstall(pkgName, sysroot, host string, pkg *Package) error {
	// Read existing cache if it exists
	cache, err := c.Read(pkgName)
	if err != nil {
		return fmt.Errorf("failed to read existing cache: %w", err)
	}
	if cache == nil {
		cache = &CacheInfo{}
	}

	// Update install-related fields
	cache.Install = pkg.Install
	cache.Env = pkg.Env
	cache.Host = host
	cache.Sysroot = sysroot

	return c.write(pkgName, cache)
}

// NeedsRebuild determines if a package needs to be rebuilt based on cache
func (c *Cache) NeedsRebuild(pkg *Package, sysroot, host string) (bool, error) {
	needs, _, err := c.needsRebuildWithReason(pkg, sysroot, host)
	return needs, err
}

// NeedsRebuildWithReason determines if a package needs to be rebuilt and returns the reason
func (c *Cache) NeedsRebuildWithReason(pkg *Package, sysroot, host string) (bool, string, error) {
	return c.needsRebuildWithReason(pkg, sysroot, host)
}

// needsRebuildWithReason determines if a package needs to be rebuilt and returns the reason (internal)
func (c *Cache) needsRebuildWithReason(pkg *Package, sysroot, host string) (bool, string, error) {
	pkgDir := filepath.Join(c.buildDir, pkg.Name)

	debugLog("Checking if %s needs rebuild...", pkg.Name)

	// Read cache
	cache, err := c.Read(pkg.Name)
	if err != nil {
		return false, "", err
	}
	if cache == nil {
		debugLog("  %s needs rebuild: no cache exists", pkg.Name)
		return true, "no cache exists", nil
	}

	// Check if URL changed
	if cache.URL != pkg.URL {
		reason := fmt.Sprintf("URL changed from %q to %q", cache.URL, pkg.URL)
		debugLog("  %s needs rebuild: %s", pkg.Name, reason)
		return true, reason, nil
	}

	// Check if build script changed
	if cache.Build != pkg.Build {
		debugLog("  %s needs rebuild: build script changed", pkg.Name)
		return true, "build script changed", nil
	}

	// Check if env vars changed
	if !stringSlicesEqual(cache.Env, pkg.Env) {
		debugLog("  %s needs rebuild: env vars changed", pkg.Name)
		return true, "env vars changed", nil
	}

	// Check if host changed
	if cache.Host != host {
		reason := fmt.Sprintf("host changed from %q to %q", cache.Host, host)
		debugLog("  %s needs rebuild: %s", pkg.Name, reason)
		return true, reason, nil
	}

	// Check if sysroot changed - this requires rebuild because build artifacts
	// contain hardcoded paths from configure scripts
	if cache.Sysroot != sysroot {
		reason := fmt.Sprintf("sysroot changed from %q to %q", cache.Sysroot, sysroot)
		debugLog("  %s needs rebuild: %s", pkg.Name, reason)
		return true, reason, nil
	}

	// Check if source directory exists
	srcDir := filepath.Join(pkgDir, sourceDir)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		debugLog("  %s needs rebuild: source directory doesn't exist", pkg.Name)
		return true, "source directory doesn't exist", nil
	}

	debugLog("  %s does not need rebuild (cache is valid)", pkg.Name)
	return false, "", nil
}

// NeedsReinstall determines if a package needs to be reinstalled (but not rebuilt)
func (c *Cache) NeedsReinstall(pkg *Package, sysroot, host string) (bool, error) {
	needs, _, err := c.needsReinstallWithReason(pkg, sysroot, host)
	return needs, err
}

// NeedsReinstallWithReason determines if a package needs to be reinstalled and returns the reason
func (c *Cache) NeedsReinstallWithReason(pkg *Package, sysroot, host string) (bool, string, error) {
	return c.needsReinstallWithReason(pkg, sysroot, host)
}

// needsReinstallWithReason determines if a package needs to be reinstalled and returns the reason (internal)
func (c *Cache) needsReinstallWithReason(pkg *Package, sysroot, host string) (bool, string, error) {
	debugLog("Checking if %s needs reinstall...", pkg.Name)

	// Read cache
	cache, err := c.Read(pkg.Name)
	if err != nil {
		return false, "", err
	}
	if cache == nil {
		debugLog("  %s needs reinstall: no cache exists", pkg.Name)
		return true, "no cache exists", nil
	}

	// Check if install script changed
	if cache.Install != pkg.Install {
		debugLog("  %s needs reinstall: install script changed", pkg.Name)
		return true, "install script changed", nil
	}

	// Check if env vars changed
	if !stringSlicesEqual(cache.Env, pkg.Env) {
		debugLog("  %s needs reinstall: env vars changed", pkg.Name)
		return true, "env vars changed", nil
	}

	// Check if host changed
	if cache.Host != host {
		reason := fmt.Sprintf("host changed from %q to %q", cache.Host, host)
		debugLog("  %s needs reinstall: %s", pkg.Name, reason)
		return true, reason, nil
	}

	// Check if sysroot changed
	if cache.Sysroot != sysroot {
		reason := fmt.Sprintf("sysroot changed from %q to %q", cache.Sysroot, sysroot)
		debugLog("  %s needs reinstall: %s", pkg.Name, reason)
		return true, reason, nil
	}

	debugLog("  %s does not need reinstall (cache is valid)", pkg.Name)
	return false, "", nil
}

// Clean removes the cache and source for a package
func (c *Cache) Clean(pkgName string) error {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	srcDir := filepath.Join(pkgDir, sourceDir)

	if err := os.RemoveAll(srcDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove source directory: %w", err)
	}

	entries, err := os.ReadDir(pkgDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read package directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != cacheFileName {
			archivePath := filepath.Join(pkgDir, entry.Name())
			if err := os.Remove(archivePath); err != nil {
				return fmt.Errorf("failed to remove archive: %w", err)
			}
		}
	}

	return nil
}

// Invalidate removes the cache file for a package
func (c *Cache) Invalidate(pkgName string) error {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	cachePath := filepath.Join(pkgDir, cacheFileName)
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache: %w", err)
	}
	return nil
}

// InvalidateDependents invalidates the cache for all packages that depend on the given package
func (c *Cache) InvalidateDependents(pkgName string, config *Config) error {
	dependents := c.findDependents(pkgName, config)

	debugLog("Package %s was rebuilt, invalidating %d dependent package(s)", pkgName, len(dependents))

	for _, dep := range dependents {
		debugLog("  Invalidating cache for %s (depends on %s)", dep, pkgName)
		if err := c.Invalidate(dep); err != nil {
			return fmt.Errorf("failed to invalidate %s: %w", dep, err)
		}
	}

	return nil
}

// findDependents returns all packages that directly or indirectly depend on the given package
func (c *Cache) findDependents(pkgName string, config *Config) []string {
	// Build a map of direct dependents
	directDependents := make(map[string][]string)
	for _, pkg := range config.Packages {
		for _, dep := range pkg.DependsOn {
			directDependents[dep] = append(directDependents[dep], pkg.Name)
		}
	}

	// Use BFS to find all transitive dependents
	visited := make(map[string]bool)
	queue := []string{pkgName}
	var result []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		// Add all direct dependents to queue and result
		for _, dep := range directDependents[current] {
			if !visited[dep] {
				queue = append(queue, dep)
				result = append(result, dep)
			}
		}
	}

	return result
}

// stringSlicesEqual compares two string slices for equality
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
