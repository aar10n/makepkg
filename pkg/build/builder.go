package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aar10n/makepkg/pkg/cache"
	"github.com/aar10n/makepkg/pkg/config"
	"github.com/aar10n/makepkg/pkg/download"
	"github.com/aar10n/makepkg/pkg/env"
	"github.com/aar10n/makepkg/pkg/logger"
)

// Result represents the result of building a package.
type Result struct {
	Package string
	Success bool
	Error   error
	Output  string
}

// BuilderConfig holds configuration options for the builder.
type BuilderConfig struct {
	Quiet          bool
	Verbose        bool
	FailFast       bool
	DryRun         bool
	AlwaysInstall  bool
	MaxConcurrency int
	MakeJobs       int
}

// Builder orchestrates the building of packages.
type Builder struct {
	*logger.Logger
	builderCfg BuilderConfig
	config     *config.Config
	envManager *env.Manager
	toolEnv    env.Env
	dryRun     bool
	buildDir   string
	sysroot    string
	host       string

	cache             cache.Cache
	downloader        download.Downloader
	buildArtifactsDir string
	results           []Result
	resultsMutex      sync.Mutex
	stopChan          chan struct{}
	stopped           bool
	stoppedMutex      sync.Mutex
	requestedPackages map[string]bool
	requiredBy        map[string][]string
	rebuiltPackages   map[string]bool
	rebuiltMutex      sync.Mutex
}

// NewBuilder creates a new Builder instance.
func NewBuilder(builderCfg BuilderConfig, cfg *config.Config, buildDir, sysroot, host, makepkgCmd string) (*Builder, error) {
	envManager := env.NewManager()
	envManager.Set("PKGS_ROOT", filepath.Dir(cfg.FilePath))
	envManager.Set("PKGS_ARCH", cfg.Toolchain.Arch)
	envManager.Set("BUILD_DIR", envManager.Subst(buildDir))
	envManager.Set("SYS_ROOT", envManager.Subst(sysroot))
	envManager.Set("MAKEPKG", makepkgCmd)
	if host != "" {
		envManager.Set("PKGS_HOST", envManager.Subst(host))
	}

	// Set up build artifacts directory
	buildArtifactsDir := filepath.Join(buildDir, "artifacts")
	if !builderCfg.DryRun {
		if err := os.MkdirAll(buildArtifactsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create build artifacts directory: %w", err)
		}
	}
	envManager.Set("BUILD_ARTIFACTS", buildArtifactsDir)

	// Substitute toolchain variables before adding to environment
	cfg.Toolchain.Subst(envManager)

	toolEnv := env.NewManager()
	cfg.Toolchain.AddToEnv(toolEnv)

	cacheInst := cache.NewCache(buildDir)
	downloader := download.NewDownloader(buildDir)

	builderLogger := logger.Default().Clone()
	if builderCfg.DryRun {
		builderLogger.SetPrefix("[DRY RUN] ")
	}

	return &Builder{
		Logger:     builderLogger,
		builderCfg: builderCfg,
		config:     cfg,
		envManager: envManager,
		toolEnv:    toolEnv,
		dryRun:     builderCfg.DryRun,
		buildDir:   buildDir,
		sysroot:    sysroot,
		host:       host,

		cache:             cacheInst,
		downloader:        downloader,
		buildArtifactsDir: buildArtifactsDir,
		results:           nil,
		stopChan:          make(chan struct{}),
		stopped:           false,
		requestedPackages: make(map[string]bool),
		requiredBy:        make(map[string][]string),
		rebuiltPackages:   make(map[string]bool),
	}, nil
}

// Build builds all packages according to the dependency order.
// If packageFilter is non-empty, only builds the specified packages (and their dependencies).
func (b *Builder) Build(ctx context.Context, packageFilter []string) error {
	b.Info("Starting build process...")
	for i := range b.config.Packages {
		b.config.Packages[i].Subst(b.envManager)
	}

	if !b.builderCfg.DryRun {
		if err := os.MkdirAll(b.sysroot, 0o755); err != nil {
			return fmt.Errorf("failed to create sysroot directory: %w", err)
		}
	}

	buildOrder, err := GetBuildOrder(b.config)
	if err != nil {
		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	filterSet := make(map[string]bool)
	if len(packageFilter) > 0 {
		for _, pkgName := range packageFilter {
			filterSet[pkgName] = true
			b.requestedPackages[pkgName] = true
		}

		for _, pkgName := range packageFilter {
			b.addDependenciesToFilter(pkgName, filterSet)
		}
	}

	b.buildRequiredByMap(filterSet)

	for _, level := range buildOrder {
		if b.isStopped() {
			b.Error("\nBuild stopped due to error (fail-fast mode)")
			return fmt.Errorf("build stopped early")
		}

		filteredLevel := level
		if len(filterSet) > 0 {
			filteredLevel = b.filterPackages(level, filterSet)
		}

		if len(filteredLevel) == 0 {
			continue
		}

		if err := b.buildLevel(ctx, filteredLevel); err != nil {
			if b.builderCfg.FailFast {
				return err
			}
			b.Warn("errors occurred in build level: %v", err)
		}
	}

	return nil
}

// Clean cleans all packages or the specified packages.
// If packageFilter is non-empty, only cleans the specified packages.
func (b *Builder) Clean(packageFilter []string) error {
	b.Info("Cleaning packages...")
	b.Info("")

	filterSet := make(map[string]bool)
	if len(packageFilter) > 0 {
		for _, pkgName := range packageFilter {
			filterSet[pkgName] = true
		}
	}

	pool := NewWorkerPool(b.builderCfg.MaxConcurrency)

	for i := range b.config.Packages {
		if len(filterSet) > 0 && !filterSet[b.config.Packages[i].Name] {
			continue
		}

		pkg := &b.config.Packages[i]
		pool.Submit(func() {
			if err := b.cleanPackage(pkg); err != nil {
				b.Warn("failed to clean %s: %v", pkg.Name, err)
			}
		})
	}

	pool.Wait()
	return nil
}

// PrintSummary prints a summary of the build results.
func (b *Builder) PrintSummary() {
	separator := strings.Repeat("=", 60)
	b.Info("")
	b.Info("%s", separator)
	b.Info("Build Summary")
	b.Info("%s", separator)

	successCount := 0
	failCount := 0

	resultMap := make(map[string]Result)
	for _, result := range b.results {
		resultMap[result.Package] = result
	}

	for _, pkg := range b.config.Packages {
		if result, ok := resultMap[pkg.Name]; ok {
			isDependency := len(b.requestedPackages) > 0 && !b.requestedPackages[pkg.Name]
			dependencyLabel := ""
			if isDependency {
				dependencyLabel = " (dependency)"
			}

			if result.Success {
				successCount++
				b.Info("✓ %s%s", result.Package, dependencyLabel)
			} else {
				failCount++
				b.Info("✗ %s%s: %v", result.Package, dependencyLabel, result.Error)
			}
		}
	}

	b.Info("%s", separator)
	b.Info("Total: %d | Success: %d | Failed: %d", len(b.results), successCount, failCount)
	b.Info("%s", separator)
}

func (b *Builder) cleanPackage(pkg *config.Package) error {
	b.Info("Cleaning %s...", pkg.Name)

	sourceDir := filepath.Join(b.buildDir, pkg.Name, "source")

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		b.Info("  No source directory found for %s, skipping", pkg.Name)
		return nil
	}

	cleanEnv := b.envManager.EnvironmentForPackage(pkg.Name, pkg.Env, b.sysroot, b.builderCfg.MakeJobs)
	if pkg.Clean != "" {
		b.Info("  Running custom clean script for %s...", pkg.Name)
		_, err := b.runScript(pkg.Name, ScriptTypeClean, pkg.Clean, cleanEnv.ToSlice())
		if err == nil {
			b.cache.Invalidate(pkg.Name)
			b.Info("  %s cleaned successfully", pkg.Name)
			return nil
		}
		b.Warn("Custom clean script failed, trying make clean...")
	}

	b.Info("  Running 'make clean' for %s...", pkg.Name)
	_, err := b.runScript(pkg.Name, ScriptTypeClean, "make clean", cleanEnv.ToSlice())
	if err == nil {
		b.cache.Invalidate(pkg.Name)
		b.Info("  %s cleaned successfully", pkg.Name)
		return nil
	}
	b.Warn("'make clean' failed, removing source directory...")

	b.Info("  Removing source directory for %s...", pkg.Name)
	if err := os.RemoveAll(sourceDir); err != nil {
		return fmt.Errorf("failed to remove source directory: %w", err)
	}

	b.cache.Invalidate(pkg.Name)
	b.Info("  %s cleaned successfully", pkg.Name)
	return nil
}

func (b *Builder) buildLevel(ctx context.Context, packageNames []string) error {
	pool := NewWorkerPool(b.builderCfg.MaxConcurrency)
	errors := make([]error, 0)
	var errorsMutex sync.Mutex

	for _, pkgName := range packageNames {
		if b.isStopped() {
			break
		}

		name := pkgName
		pool.SubmitWithStop(func() {
			if b.isStopped() {
				return
			}

			pkg := b.config.GetPackageByName(name)
			if pkg == nil {
				errorsMutex.Lock()
				errors = append(errors, fmt.Errorf("package %s not found", name))
				errorsMutex.Unlock()
				if b.builderCfg.FailFast {
					b.stop()
				}
				return
			}

			if err := b.buildPackage(ctx, pkg); err != nil {
				errorsMutex.Lock()
				errors = append(errors, err)
				errorsMutex.Unlock()
				if b.builderCfg.FailFast {
					b.stop()
				}
			}
		}, b.stopChan)
	}

	pool.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("build errors: %v", errors)
	}
	return nil
}

func (b *Builder) buildPackage(ctx context.Context, pkg *config.Package) error {
	requiredBy := b.requiredBy[pkg.Name]
	b.Info("Building %s%s...", pkg.Name, formatRequiredBy(requiredBy))

	needsRebuild, err := b.cache.NeedsRebuild(pkg, b.sysroot, b.host)
	if err != nil {
		return fmt.Errorf("failed to check cache for %s: %w", pkg.Name, err)
	}

	needsReinstall := b.builderCfg.AlwaysInstall
	if !needsReinstall {
		needsReinstall, err = b.cache.NeedsReinstall(pkg, b.sysroot, b.host)
		if err != nil {
			return fmt.Errorf("failed to check reinstall cache for %s: %w", pkg.Name, err)
		}
	}

	if !needsRebuild && !needsReinstall {
		b.Info("  %s is up to date, skipping", pkg.Name)
		b.recordResult(pkg.Name, true, nil, "")
		return nil
	}

	// Clean up package-specific build artifacts directory
	pkgArtifactsDir := filepath.Join(b.buildArtifactsDir, pkg.Name)
	if !b.builderCfg.DryRun {
		if err := os.RemoveAll(pkgArtifactsDir); err != nil {
			b.Warn("  Failed to clean artifacts for %s: %v", pkg.Name, err)
		} else if _, err := os.Stat(pkgArtifactsDir); err == nil {
			b.Debug("  Cleaned artifacts directory for %s", pkg.Name)
		}

		if err := os.MkdirAll(pkgArtifactsDir, 0755); err != nil {
			b.Warn("  Failed to create artifacts directory for %s: %v", pkg.Name, err)
		}
	}

	var buildOutput string
	var installOutput string
	sourceDir := filepath.Join(b.buildDir, pkg.Name, "source")

	pkgEnv := b.envManager.EnvironmentForPackage(pkg.Name, pkg.Env, b.sysroot, b.builderCfg.MakeJobs)
	if !pkg.Native {
		b.toolEnv.AddToEnv(pkgEnv)
	}
	if needsRebuild {
		info, _ := b.cache.Read(pkg.Name)
		if info != nil && info.URL != pkg.URL {
			b.Info("  URL changed for %s, cleaning old build", pkg.Name)
			if !b.builderCfg.DryRun {
				if err := b.cache.Clean(pkg.Name); err != nil {
					return fmt.Errorf("failed to clean info for %s: %w", pkg.Name, err)
				}
			} else {
				b.Info("Would clean old build for %s due to URL change", pkg.Name)
			}
		}

		if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
			if !b.builderCfg.DryRun {
				b.Info("  Downloading %s...", pkg.Name)
				if err := b.downloader.Download(ctx, pkg.Name, pkg.URL); err != nil {
					b.recordResult(pkg.Name, false, err, "")
					return fmt.Errorf("failed to download %s: %w", pkg.Name, err)
				}
				if err := b.downloader.Extract(pkg.Name, pkg.URL); err != nil {
					b.recordResult(pkg.Name, false, err, "")
					return fmt.Errorf("failed to extract %s: %w", pkg.Name, err)
				}
			} else {
				b.Info("  [DRY RUN] Would download and extract %s", pkg.Name)
			}
		}

		b.Info("  Compiling %s...", pkg.Name)
		b.Debug("=== Build environment for %s ===", pkg.Name)
		logEnvironment(pkgEnv.ToSlice())
		if !b.builderCfg.DryRun {
			buildOutputTmp, err := b.runScript(pkg.Name, ScriptTypeBuild, pkg.Build, pkgEnv.ToSlice())
			if err != nil {
				b.recordResult(pkg.Name, false, err, buildOutputTmp)
				return fmt.Errorf("failed to build %s: %w", pkg.Name, err)
			}
			buildOutput = buildOutputTmp
			if err := b.cache.WriteBuild(pkg.Name, b.sysroot, b.host, pkg); err != nil {
				b.Warn("failed to write build info for %s: %v", pkg.Name, err)
			}

			if err := b.cache.InvalidateDependents(pkg.Name, b.config); err != nil {
				b.Warn("failed to invalidate dependents for %s: %v", pkg.Name, err)
			}
		} else {
			b.Info("  [DRY RUN] Would run build commands:")
			for _, line := range strings.Split(pkg.Build, "\n") {
				if strings.TrimSpace(line) != "" {
					b.Info("    %s", line)
				}
			}
			b.rebuiltMutex.Lock()
			b.rebuiltPackages[pkg.Name] = true
			b.rebuiltMutex.Unlock()
		}
	} else {
		b.Info("  %s is already built, reinstalling to new sysroot...", pkg.Name)
	}

	b.Info("  Installing %s...", pkg.Name)
	b.Debug("=== Install environment for %s ===", pkg.Name)
	logEnvironment(pkgEnv.ToSlice())
	if !b.builderCfg.DryRun {
		installOutput, err = b.runScript(pkg.Name, ScriptTypeInstall, pkg.Install, pkgEnv.ToSlice())
		if err != nil {
			b.recordResult(pkg.Name, false, err, buildOutput+"\n"+installOutput)
			return fmt.Errorf("failed to install %s: %w", pkg.Name, err)
		}

		if err := b.cache.WriteInstall(pkg.Name, b.sysroot, b.host, pkg); err != nil {
			b.Warn("failed to write install cache for %s: %v", pkg.Name, err)
		}
	} else {
		b.Info("  [DRY RUN] Would run install commands:")
		for _, line := range strings.Split(pkg.Install, "\n") {
			if strings.TrimSpace(line) != "" {
				b.Info("    %s", line)
			}
		}
	}

	fullOutput := buildOutput + "\n" + installOutput
	b.recordResult(pkg.Name, true, nil, fullOutput)
	b.Info("  %s built successfully", pkg.Name)
	return nil
}

func (b *Builder) runScript(pkgName string, scriptType ScriptType, script string, env []string) (string, error) {
	sourceDir := filepath.Join(b.buildDir, pkgName, "source")
	b.Debug("Running script in directory: %s", sourceDir)
	b.Debug("Script content:\n%s", script)

	fullScript := GetScriptPreamble(scriptType) + script
	cmd := exec.Command("bash", "-c", fullScript)
	cmd.Dir = sourceDir
	cmd.Env = env

	var outputBuf bytes.Buffer
	var combinedOutput io.Writer = &outputBuf

	if !b.builderCfg.Quiet {
		combinedOutput = io.MultiWriter(&outputBuf, os.Stdout)
	}

	cmd.Stdout = combinedOutput
	cmd.Stderr = combinedOutput

	b.Debug("Executing command: bash -c <script>")
	err := cmd.Run()
	if err != nil {
		b.Debug("Command failed with error: %v", err)
	} else {
		b.Debug("Command completed successfully")
	}
	return outputBuf.String(), err
}

func (b *Builder) recordResult(pkgName string, success bool, err error, output string) {
	b.resultsMutex.Lock()
	defer b.resultsMutex.Unlock()

	b.results = append(b.results, Result{
		Package: pkgName,
		Success: success,
		Error:   err,
		Output:  output,
	})
}

func (b *Builder) stop() {
	b.stoppedMutex.Lock()
	defer b.stoppedMutex.Unlock()

	if !b.stopped {
		b.stopped = true
		close(b.stopChan)
	}
}

func (b *Builder) isStopped() bool {
	b.stoppedMutex.Lock()
	defer b.stoppedMutex.Unlock()
	return b.stopped
}

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

func (b *Builder) filterPackages(packages []string, filterSet map[string]bool) []string {
	filtered := make([]string, 0, len(packages))
	for _, pkgName := range packages {
		if filterSet[pkgName] {
			filtered = append(filtered, pkgName)
		}
	}
	return filtered
}

func (b *Builder) buildRequiredByMap(filterSet map[string]bool) {
	for _, pkg := range b.config.Packages {
		if len(filterSet) > 0 && !filterSet[pkg.Name] {
			continue
		}

		for _, dep := range pkg.DependsOn {
			if len(filterSet) == 0 || filterSet[dep] {
				b.requiredBy[dep] = append(b.requiredBy[dep], pkg.Name)
			}
		}
	}
}

func formatRequiredBy(requiredBy []string) string {
	if len(requiredBy) == 0 {
		return ""
	}
	if len(requiredBy) > 6 {
		displayed := strings.Join(requiredBy[:5], ", ")
		remaining := len(requiredBy) - 5
		return fmt.Sprintf(" (required by %s, and +%d more)", displayed, remaining)
	}
	return fmt.Sprintf(" (required by %s)", strings.Join(requiredBy, ", "))
}

func logEnvironment(env []string) {
	importantVars := []string{
		"PATH", "CC", "CXX", "AR", "LD", "AS", "NM", "RANLIB", "STRIP",
		"CFLAGS", "CXXFLAGS", "LDFLAGS", "CPPFLAGS",
		"PKG_CONFIG_PATH", "PKG_CONFIG_SYSROOT_DIR",
		"SYS_ROOT", "INSTALL_ROOT", "PKGS_HOST",
		"LIBRARY_PATH", "LD_LIBRARY_PATH",
		"BUILD_ARTIFACTS", "MAKEPKG",
	}

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	for _, varName := range importantVars {
		if value, ok := envMap[varName]; ok {
			logger.Debug("  %s=%s", varName, value)
		}
	}
}
