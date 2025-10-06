package pkg

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// BuildResult represents the result of building a package
type BuildResult struct {
	Package string
	Success bool
	Error   error
	Output  string
}

// BuilderConfig holds configuration options for the builder
type BuilderConfig struct {
	Quiet          bool
	Verbose        bool
	FailFast       bool
	DryRun         bool
	MaxConcurrency int
}

// Builder orchestrates the building of packages
type Builder struct {
	config            *Config
	buildDir          string
	sysroot           string
	host              string
	toolchain         *Toolchain
	envSubst          *EnvSubst
	cache             *Cache
	cfg               BuilderConfig
	results           []BuildResult
	resultsMutex      sync.Mutex
	stopChan          chan struct{}
	stopped           bool
	stoppedMutex      sync.Mutex
	requestedPackages map[string]bool     // Tracks explicitly requested packages vs dependencies
	requiredBy        map[string][]string // Maps package name to list of packages that require it
}

// NewBuilder creates a new Builder instance
func NewBuilder(config *Config, buildDir, sysroot, host string, cfg BuilderConfig) (*Builder, error) {
	// Enable verbose logging globally if requested
	VerboseLogging = cfg.Verbose

	// Build environment variable map, excluding empty values
	envMap := map[string]string{
		"PKGS_ROOT": config.PackagesRoot,
		"BUILD_DIR": buildDir,
		"SYS_ROOT":  sysroot,
	}

	// Only add PKGS_HOST if it was specified
	if host != "" {
		envMap["PKGS_HOST"] = host
	}

	envSubst := NewEnvSubst(WithEnvMap(envMap))

	toolchain, err := NewToolchainFromConfig(config, envSubst)
	if err != nil {
		return nil, err
	}

	cache := NewCache(buildDir)

	return &Builder{
		config:            config,
		buildDir:          buildDir,
		sysroot:           sysroot,
		host:              host,
		toolchain:         toolchain,
		envSubst:          envSubst,
		cache:             cache,
		cfg:               cfg,
		results:           nil,
		stopChan:          make(chan struct{}),
		stopped:           false,
		requestedPackages: make(map[string]bool),
		requiredBy:        make(map[string][]string),
	}, nil
}

// Build builds all packages according to the dependency order
// If packageFilter is non-empty, only builds the specified packages (and their dependencies)
func (b *Builder) Build(packageFilter []string) error {
	// Print toolchain
	fmt.Printf("%s\n", b.toolchain.String())

	// Resolve build order
	buildOrder, err := BuildOrder(b.config)
	if err != nil {
		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	// Create filter set for quick lookup
	filterSet := make(map[string]bool)
	if len(packageFilter) > 0 {
		// Track explicitly requested packages
		for _, pkgName := range packageFilter {
			filterSet[pkgName] = true
			b.requestedPackages[pkgName] = true
		}

		// Add dependencies of filtered packages to the filter set
		for _, pkgName := range packageFilter {
			b.addDependenciesToFilter(pkgName, filterSet)
		}
	}

	// Build reverse dependency map to show which packages require each package
	b.buildRequiredByMap(filterSet)

	// Build packages level by level
	for _, level := range buildOrder {
		// Check if we should stop
		if b.isStopped() {
			fmt.Fprintf(os.Stderr, "\nBuild stopped due to error (fail-fast mode)\n")
			return fmt.Errorf("build stopped early")
		}

		// Filter the level if needed
		filteredLevel := level
		if len(filterSet) > 0 {
			filteredLevel = b.filterPackages(level, filterSet)
		}

		if len(filteredLevel) == 0 {
			continue
		}

		if err := b.buildLevel(filteredLevel); err != nil {
			if b.cfg.FailFast {
				return err
			}
			// Don't stop on error, continue building other packages
			fmt.Fprintf(os.Stderr, "Warning: errors occurred in build level: %v\n", err)
		}
	}

	return nil
}

// buildLevel builds all packages in a level concurrently
func (b *Builder) buildLevel(packageNames []string) error {
	// Create semaphore for concurrency control
	sem := make(chan struct{}, b.cfg.MaxConcurrency)
	if b.cfg.MaxConcurrency <= 0 {
		sem = make(chan struct{}, 1) // Sequential execution
	}

	var wg sync.WaitGroup
	errors := make([]error, 0)
	var errorsMutex sync.Mutex

	for _, pkgName := range packageNames {
		// Check if we should stop before starting new builds
		if b.isStopped() {
			break
		}

		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			// Check again before acquiring semaphore
			if b.isStopped() {
				return
			}

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-b.stopChan:
				return
			}

			// Check once more before building
			if b.isStopped() {
				return
			}

			pkg := b.config.GetPackageByName(name)
			if pkg == nil {
				errorsMutex.Lock()
				errors = append(errors, fmt.Errorf("package %s not found", name))
				errorsMutex.Unlock()
				if b.cfg.FailFast {
					b.stop()
				}
				return
			}

			if err := b.buildPackage(pkg); err != nil {
				errorsMutex.Lock()
				errors = append(errors, err)
				errorsMutex.Unlock()
				if b.cfg.FailFast {
					b.stop()
				}
			}
		}(pkgName)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("build errors: %v", errors)
	}
	return nil
}

// buildPackage builds a single package
func (b *Builder) buildPackage(pkg *Package) error {
	pkg.URL = b.envSubst.Subst(pkg.URL)

	// Build the "required by" message if there are dependents
	requiredByMsg := ""
	if requiredBy, ok := b.requiredBy[pkg.Name]; ok && len(requiredBy) > 0 {
		if len(requiredBy) > 6 {
			displayed := strings.Join(requiredBy[:5], ", ")
			remaining := len(requiredBy) - 5
			requiredByMsg = fmt.Sprintf(" (required by %s, and +%d more)", displayed, remaining)
		} else {
			requiredByMsg = fmt.Sprintf(" (required by %s)", strings.Join(requiredBy, ", "))
		}
	}
	fmt.Printf("Building %s%s...\n", pkg.Name, requiredByMsg)

	// Check if rebuild is needed
	needsRebuild, err := b.cache.NeedsRebuild(pkg, b.sysroot, b.host)
	if err != nil {
		return fmt.Errorf("failed to check cache for %s: %w", pkg.Name, err)
	}

	// Check if only reinstall is needed (sysroot changed but build is same)
	needsReinstall, err := b.cache.NeedsReinstall(pkg, b.sysroot, b.host)
	if err != nil {
		return fmt.Errorf("failed to check reinstall cache for %s: %w", pkg.Name, err)
	}

	// Handle dry-run mode with accurate cache state reporting
	if b.cfg.DryRun {
		if !needsRebuild && !needsReinstall {
			fmt.Printf("  [DRY RUN] %s is up to date, would skip\n", pkg.Name)
		} else if !needsRebuild && needsReinstall {
			// Get the reason for reinstall
			_, reason, _ := b.cache.NeedsReinstallWithReason(pkg, b.sysroot, b.host)
			if reason != "" {
				fmt.Printf("  [DRY RUN] %s is already built, would reinstall only (%s)\n", pkg.Name, reason)
			} else {
				fmt.Printf("  [DRY RUN] %s is already built, would reinstall only\n", pkg.Name)
			}
			fmt.Printf("  [DRY RUN] Would run install commands:\n")
			for _, line := range strings.Split(pkg.InstallScript(b.envSubst), "\n") {
				if strings.TrimSpace(line) != "" {
					fmt.Printf("    %s\n", line)
				}
			}
		} else {
			// Get the reason for rebuild
			_, reason, _ := b.cache.NeedsRebuildWithReason(pkg, b.sysroot, b.host)
			sourceDir := filepath.Join(b.buildDir, pkg.Name, "source")
			if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
				fmt.Printf("  [DRY RUN] Would download from: %s\n", pkg.URL)
			} else {
				fmt.Printf("  [DRY RUN] Source already downloaded\n")
			}
			if reason != "" {
				fmt.Printf("  [DRY RUN] Would rebuild (%s)\n", reason)
			}
			fmt.Printf("  [DRY RUN] Would run build commands:\n")
			for _, line := range strings.Split(pkg.BuildScript(b.envSubst), "\n") {
				if strings.TrimSpace(line) != "" {
					fmt.Printf("    %s\n", line)
				}
			}
			fmt.Printf("  [DRY RUN] Would run install commands:\n")
			for _, line := range strings.Split(pkg.InstallScript(b.envSubst), "\n") {
				if strings.TrimSpace(line) != "" {
					fmt.Printf("    %s\n", line)
				}
			}
		}
		b.recordResult(pkg.Name, true, nil, "")
		return nil
	}

	if !needsRebuild && !needsReinstall {
		fmt.Printf("  %s is up to date, skipping\n", pkg.Name)
		b.recordResult(pkg.Name, true, nil, "")
		return nil
	}

	var output string
	sourceDir := filepath.Join(b.buildDir, pkg.Name, "source")

	if needsRebuild {
		cache, _ := b.cache.Read(pkg.Name)
		if cache != nil && cache.URL != pkg.URL {
			fmt.Printf("  URL changed for %s, cleaning old build\n", pkg.Name)
			if err := b.cache.Clean(pkg.Name); err != nil {
				return fmt.Errorf("failed to clean cache for %s: %w", pkg.Name, err)
			}
		}

		if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
			fmt.Printf("  Downloading %s...\n", pkg.Name)
			if err := DownloadAndExtract(b.buildDir, pkg.Name, pkg.URL); err != nil {
				b.recordResult(pkg.Name, false, err, "")
				return fmt.Errorf("failed to download %s: %w", pkg.Name, err)
			}
		}

		fmt.Printf("  Compiling %s...\n", pkg.Name)
		buildEnv := BuildEnvironment(pkg, b.sysroot, b.toolchain, b.envSubst)
		debugLog("=== Build environment for %s ===", pkg.Name)
		logEnvironment(buildEnv)
		buildOutput, err := b.runScript(pkg.Name, pkg.BuildScript(b.envSubst), buildEnv)
		if err != nil {
			b.recordResult(pkg.Name, false, err, buildOutput)
			return fmt.Errorf("failed to build %s: %w", pkg.Name, err)
		}
		output = buildOutput

		if err := b.cache.WriteBuild(pkg.Name, b.sysroot, b.host, pkg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write build cache for %s: %v\n", pkg.Name, err)
		}

		if err := b.cache.InvalidateDependents(pkg.Name, b.config); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to invalidate dependents for %s: %v\n", pkg.Name, err)
		}
	} else {
		fmt.Printf("  %s is already built, reinstalling to new sysroot...\n", pkg.Name)
	}

	fmt.Printf("  Installing %s...\n", pkg.Name)
	installEnv := InstallEnvironment(pkg, b.sysroot, b.toolchain, b.envSubst)
	debugLog("=== Install environment for %s ===", pkg.Name)
	logEnvironment(installEnv)
	installOutput, err := b.runScript(pkg.Name, pkg.InstallScript(b.envSubst), installEnv)
	if err != nil {
		b.recordResult(pkg.Name, false, err, output+"\n"+installOutput)
		return fmt.Errorf("failed to install %s: %w", pkg.Name, err)
	}

	if err := b.cache.WriteInstall(pkg.Name, b.sysroot, b.host, pkg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write install cache for %s: %v\n", pkg.Name, err)
	}

	fullOutput := output + "\n" + installOutput
	b.recordResult(pkg.Name, true, nil, fullOutput)
	fmt.Printf("  %s built successfully\n", pkg.Name)
	return nil
}

// runScript executes a bash script in the package source directory
func (b *Builder) runScript(pkgName, script string, env []string) (string, error) {
	sourceDir := filepath.Join(b.buildDir, pkgName, "source")

	debugLog("Running script in directory: %s", sourceDir)
	debugLog("Script content:\n%s", script)

	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = sourceDir
	cmd.Env = env

	var outputBuf bytes.Buffer
	var combinedOutput io.Writer = &outputBuf

	// If not quiet, also output to stdout/stderr
	if !b.cfg.Quiet {
		combinedOutput = io.MultiWriter(&outputBuf, os.Stdout)
	}

	cmd.Stdout = combinedOutput
	cmd.Stderr = combinedOutput

	debugLog("Executing command: bash -c <script>")
	err := cmd.Run()
	if err != nil {
		debugLog("Command failed with error: %v", err)
	} else {
		debugLog("Command completed successfully")
	}
	return outputBuf.String(), err
}

// recordResult records the build result for a package
func (b *Builder) recordResult(pkgName string, success bool, err error, output string) {
	b.resultsMutex.Lock()
	defer b.resultsMutex.Unlock()

	b.results = append(b.results, BuildResult{
		Package: pkgName,
		Success: success,
		Error:   err,
		Output:  output,
	})
}

// stop signals all goroutines to stop
func (b *Builder) stop() {
	b.stoppedMutex.Lock()
	defer b.stoppedMutex.Unlock()

	if !b.stopped {
		b.stopped = true
		close(b.stopChan)
	}
}

// isStopped checks if the builder has been stopped
func (b *Builder) isStopped() bool {
	b.stoppedMutex.Lock()
	defer b.stoppedMutex.Unlock()
	return b.stopped
}

// addDependenciesToFilter recursively adds all dependencies of a package to the filter set
func (b *Builder) addDependenciesToFilter(pkgName string, filterSet map[string]bool) {
	pkg := b.config.GetPackageByName(pkgName)
	if pkg == nil {
		return
	}

	for _, dep := range pkg.DependsOn {
		if !filterSet[dep] {
			filterSet[dep] = true
			b.addDependenciesToFilter(dep, filterSet)
		}
	}
}

// filterPackages filters a list of package names based on the filter set
func (b *Builder) filterPackages(packages []string, filterSet map[string]bool) []string {
	filtered := make([]string, 0, len(packages))
	for _, pkgName := range packages {
		if filterSet[pkgName] {
			filtered = append(filtered, pkgName)
		}
	}
	return filtered
}

// buildRequiredByMap builds a reverse dependency map showing which packages require each package
func (b *Builder) buildRequiredByMap(filterSet map[string]bool) {
	for _, pkg := range b.config.Packages {
		// If we have a filter, only consider packages in the filter set
		if len(filterSet) > 0 && !filterSet[pkg.Name] {
			continue
		}

		for _, dep := range pkg.DependsOn {
			// Only track if the dependency is also in our build set (or no filter is active)
			if len(filterSet) == 0 || filterSet[dep] {
				b.requiredBy[dep] = append(b.requiredBy[dep], pkg.Name)
			}
		}
	}
}

// Clean cleans all packages or the specified packages
// If packageFilter is non-empty, only cleans the specified packages
func (b *Builder) Clean(packageFilter []string) error {
	fmt.Printf("Cleaning packages...\n\n")

	// Create filter set for quick lookup
	filterSet := make(map[string]bool)
	if len(packageFilter) > 0 {
		for _, pkgName := range packageFilter {
			filterSet[pkgName] = true
		}
	}

	// Create semaphore for concurrency control
	sem := make(chan struct{}, b.cfg.MaxConcurrency)
	if b.cfg.MaxConcurrency <= 0 {
		sem = make(chan struct{}, 1) // Sequential execution
	}

	var wg sync.WaitGroup

	for i := range b.config.Packages {
		// Skip if filter is active and this package is not in the filter
		if len(filterSet) > 0 && !filterSet[b.config.Packages[i].Name] {
			continue
		}

		wg.Add(1)
		go func(pkg *Package) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := b.cleanPackage(pkg); err != nil {
				// Ignore errors and continue
				fmt.Fprintf(os.Stderr, "  Warning: failed to clean %s: %v\n", pkg.Name, err)
			}
		}(&b.config.Packages[i])
	}

	wg.Wait()

	return nil
}

// cleanPackage cleans a single package
func (b *Builder) cleanPackage(pkg *Package) error {
	fmt.Printf("Cleaning %s...\n", pkg.Name)

	sourceDir := filepath.Join(b.buildDir, pkg.Name, "source")

	// Check if source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		fmt.Printf("  No source directory found for %s, skipping\n", pkg.Name)
		return nil
	}

	// If package has a clean script, try running it
	if pkg.Clean != "" {
		fmt.Printf("  Running custom clean script for %s...\n", pkg.Name)
		cleanEnv := BuildEnvironment(pkg, b.sysroot, b.toolchain, b.envSubst)
		_, err := b.runScript(pkg.Name, pkg.CleanScript(b.envSubst), cleanEnv)
		if err == nil {
			b.cache.Invalidate(pkg.Name)
			fmt.Printf("  %s cleaned successfully\n", pkg.Name)
			return nil
		}
		fmt.Fprintf(os.Stderr, "  Custom clean script failed, trying make clean...\n")
	}

	// Try running make clean
	fmt.Printf("  Running 'make clean' for %s...\n", pkg.Name)
	makeEnv := BuildEnvironment(pkg, b.sysroot, b.toolchain, b.envSubst)
	_, err := b.runScript(pkg.Name, "make clean", makeEnv)
	if err == nil {
		b.cache.Invalidate(pkg.Name)
		fmt.Printf("  %s cleaned successfully\n", pkg.Name)
		return nil
	}
	fmt.Fprintf(os.Stderr, "  'make clean' failed, removing source directory...\n")

	// Remove the source directory
	fmt.Printf("  Removing source directory for %s...\n", pkg.Name)
	if err := os.RemoveAll(sourceDir); err != nil {
		return fmt.Errorf("failed to remove source directory: %w", err)
	}

	b.cache.Invalidate(pkg.Name)
	fmt.Printf("  %s cleaned successfully\n", pkg.Name)
	return nil
}

// PrintSummary prints a summary of the build results
func (b *Builder) PrintSummary() {
	separator := strings.Repeat("=", 60)
	fmt.Println("\n" + separator)
	fmt.Println("Build Summary")
	fmt.Println(separator)

	successCount := 0
	failCount := 0

	// Create a map of package name to result for quick lookup
	resultMap := make(map[string]BuildResult)
	for _, result := range b.results {
		resultMap[result.Package] = result
	}

	// Print results in the order they appear in the config file
	for _, pkg := range b.config.Packages {
		if result, ok := resultMap[pkg.Name]; ok {
			isDependency := len(b.requestedPackages) > 0 && !b.requestedPackages[pkg.Name]
			dependencyLabel := ""
			if isDependency {
				dependencyLabel = " (dependency)"
			}

			if result.Success {
				successCount++
				fmt.Printf("✓ %s%s\n", result.Package, dependencyLabel)
			} else {
				failCount++
				fmt.Printf("✗ %s%s: %v\n", result.Package, dependencyLabel, result.Error)
			}
		}
	}

	fmt.Println(separator)
	fmt.Printf("Total: %d | Success: %d | Failed: %d\n", len(b.results), successCount, failCount)
	fmt.Println(separator)
}

// logEnvironment logs important environment variables when verbose logging is enabled
func logEnvironment(env []string) {
	if !VerboseLogging {
		return
	}

	// List of important environment variables to log
	importantVars := []string{
		"PATH", "CC", "CXX", "AR", "LD", "AS", "NM", "RANLIB", "STRIP",
		"CFLAGS", "CXXFLAGS", "LDFLAGS", "CPPFLAGS",
		"PKG_CONFIG_PATH", "PKG_CONFIG_SYSROOT_DIR",
		"SYS_ROOT", "INSTALL_ROOT", "PKGS_HOST",
		"LIBRARY_PATH", "LD_LIBRARY_PATH",
	}

	// Create a map for quick lookup
	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Log important variables
	for _, varName := range importantVars {
		if value, ok := envMap[varName]; ok {
			debugLog("  %s=%s", varName, value)
		}
	}
}
