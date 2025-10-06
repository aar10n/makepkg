package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aar10n/makepkg/pkg/config"
	"github.com/aar10n/makepkg/pkg/logger"
)

const (
	cacheFileName = "makepkg.json"
	sourceDir     = "source"
)

// Info stores the cached build information for a package.
type Info struct {
	URL     string   `json:"url"`
	Build   string   `json:"build"`
	Install string   `json:"install"`
	Env     []string `json:"env"`
	Host    string   `json:"host"`
	Sysroot string   `json:"sysroot"`
}

type Cache interface {
	Read(pkgName string) (*Info, error)
	WriteBuild(pkgName, sysroot, host string, pkg *config.Package) error
	WriteInstall(pkgName, sysroot, host string, pkg *config.Package) error
	NeedsRebuild(pkg *config.Package, sysroot, host string) (bool, error)
	NeedsReinstall(pkg *config.Package, sysroot, host string) (bool, error)
	Clean(pkgName string) error
	Invalidate(pkgName string) error
	InvalidateDependents(pkgName string, cfg *config.Config) error
}

type cache struct {
	buildDir string
}

// NewCache creates a new cache instance.
func NewCache(buildDir string) Cache {
	return &cache{
		buildDir: buildDir,
	}
}

// Read reads the cached build information for a package.
func (c *cache) Read(pkgName string) (*Info, error) {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	cachePath := filepath.Join(pkgDir, cacheFileName)

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache: %w", err)
	}

	var cache Info
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}

	return &cache, nil
}

// WriteBuild writes just the build step cache.
func (c *cache) WriteBuild(pkgName, sysroot, host string, pkg *config.Package) error {
	cache, err := c.Read(pkgName)
	if err != nil {
		return fmt.Errorf("failed to read existing cache: %w", err)
	}
	if cache == nil {
		cache = &Info{}
	}

	cache.URL = pkg.URL
	cache.Build = pkg.Build
	cache.Env = pkg.Env
	cache.Host = host
	cache.Sysroot = sysroot

	return c.write(pkgName, cache)
}

// WriteInstall writes just the installation step cache.
func (c *cache) WriteInstall(pkgName, sysroot, host string, pkg *config.Package) error {
	cache, err := c.Read(pkgName)
	if err != nil {
		return fmt.Errorf("failed to read existing cache: %w", err)
	}
	if cache == nil {
		cache = &Info{}
	}

	cache.Install = pkg.Install
	cache.Env = pkg.Env
	cache.Host = host
	cache.Sysroot = sysroot

	return c.write(pkgName, cache)
}

// NeedsRebuild determines if a package needs to be rebuilt based on cache.
func (c *cache) NeedsRebuild(pkg *config.Package, sysroot, host string) (bool, error) {
	needs, _, err := c.needsRebuildWithReason(pkg, sysroot, host)
	return needs, err
}

// NeedsReinstall determines if a package needs to be reinstalled (but not rebuilt).
func (c *cache) NeedsReinstall(pkg *config.Package, sysroot, host string) (bool, error) {
	needs, _, err := c.needsReinstallWithReason(pkg, sysroot, host)
	return needs, err
}

// Clean removes the cache and source for a package.
func (c *cache) Clean(pkgName string) error {
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

// Invalidate removes the cache file for a package.
func (c *cache) Invalidate(pkgName string) error {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	cachePath := filepath.Join(pkgDir, cacheFileName)
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache: %w", err)
	}
	return nil
}

// InvalidateDependents invalidates the cache for all packages that depend on the given package.
func (c *cache) InvalidateDependents(pkgName string, cfg *config.Config) error {
	dependents := c.findDependents(pkgName, cfg)

	logger.Debug("Package %s was rebuilt, invalidating %d dependent package(s)", pkgName, len(dependents))

	for _, dep := range dependents {
		logger.Debug("  Invalidating cache for %s (depends on %s)", dep, pkgName)
		if err := c.Invalidate(dep); err != nil {
			return fmt.Errorf("failed to invalidate %s: %w", dep, err)
		}
	}

	return nil
}

func (c *cache) write(pkgName string, cache *Info) error {
	pkgDir := filepath.Join(c.buildDir, pkgName)
	cachePath := filepath.Join(pkgDir, cacheFileName)

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

func (c *cache) checkCommonCacheChanges(cache *Info, pkg *config.Package, sysroot, host string) (bool, string) {
	if !stringSlicesEqual(cache.Env, pkg.Env) {
		return true, "env vars changed"
	}

	if cache.Host != host {
		return true, fmt.Sprintf("host changed from %q to %q", cache.Host, host)
	}

	if cache.Sysroot != sysroot {
		return true, fmt.Sprintf("sysroot changed from %q to %q", cache.Sysroot, sysroot)
	}

	return false, ""
}

func (c *cache) needsRebuildWithReason(pkg *config.Package, sysroot, host string) (bool, string, error) {
	pkgDir := filepath.Join(c.buildDir, pkg.Name)

	logger.Debug("Checking if %s needs rebuild...", pkg.Name)

	cache, err := c.Read(pkg.Name)
	if err != nil {
		return false, "", err
	}
	if cache == nil {
		logger.Debug("  %s needs rebuild: no cache exists", pkg.Name)
		return true, "no cache exists", nil
	}

	if cache.URL != pkg.URL {
		reason := fmt.Sprintf("URL changed from %q to %q", cache.URL, pkg.URL)
		logger.Debug("  %s needs rebuild: %s", pkg.Name, reason)
		return true, reason, nil
	}

	if cache.Build != pkg.Build {
		logger.Debug("  %s needs rebuild: build script changed", pkg.Name)
		return true, "build script changed", nil
	}

	if changed, reason := c.checkCommonCacheChanges(cache, pkg, sysroot, host); changed {
		logger.Debug("  %s needs rebuild: %s", pkg.Name, reason)
		return true, reason, nil
	}

	srcDir := filepath.Join(pkgDir, sourceDir)
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		logger.Debug("  %s needs rebuild: source directory doesn't exist", pkg.Name)
		return true, "source directory doesn't exist", nil
	}

	logger.Debug("  %s does not need rebuild (cache is valid)", pkg.Name)
	return false, "", nil
}

func (c *cache) needsReinstallWithReason(pkg *config.Package, sysroot, host string) (bool, string, error) {
	logger.Debug("Checking if %s needs reinstall...", pkg.Name)

	cache, err := c.Read(pkg.Name)
	if err != nil {
		return false, "", err
	}
	if cache == nil {
		logger.Debug("  %s needs reinstall: no cache exists", pkg.Name)
		return true, "no cache exists", nil
	}

	if cache.Install != pkg.Install {
		logger.Debug("  %s needs reinstall: install script changed", pkg.Name)
		return true, "install script changed", nil
	}

	if changed, reason := c.checkCommonCacheChanges(cache, pkg, sysroot, host); changed {
		logger.Debug("  %s needs reinstall: %s", pkg.Name, reason)
		return true, reason, nil
	}

	logger.Debug("  %s does not need reinstall (cache is valid)", pkg.Name)
	return false, "", nil
}

func (c *cache) findDependents(pkgName string, cfg *config.Config) []string {
	directDependents := make(map[string][]string)
	for _, pkg := range cfg.Packages {
		for _, dep := range pkg.DependsOn {
			directDependents[dep] = append(directDependents[dep], pkg.Name)
		}
	}

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

		for _, dep := range directDependents[current] {
			if !visited[dep] {
				queue = append(queue, dep)
				result = append(result, dep)
			}
		}
	}

	return result
}

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
