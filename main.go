package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/pflag"

	"github.com/aar10n/makepkg/pkg/build"
	"github.com/aar10n/makepkg/pkg/config"
	"github.com/aar10n/makepkg/pkg/logger"
)

var (
	signalHandler = make(chan struct{})
	version       = "dev"     // set by goreleaser
	commit        = "unknown" // set by goreleaser
	date          = "unknown" // set by goreleaser
)

func main() {
	f := parseFlags()

	if f.showVersion {
		fmt.Printf("makepkg %s\n", version)
		if commit != "unknown" {
			fmt.Printf("commit: %s\n", commit)
		}
		if date != "unknown" {
			fmt.Printf("built: %s\n", date)
		}
		os.Exit(0)
	}

	configPath := f.configFile
	packageFilter := pflag.Args()

	if configPath != "" {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			logger.Errorf("configuration file %s not found", configPath)
			os.Exit(1)
		}
	}

	buildDir := f.builddir
	if !filepath.IsAbs(buildDir) {
		absPath, err := filepath.Abs(buildDir)
		if err != nil {
			logger.Errorf("resolving build directory: %v", err)
			os.Exit(1)
		}
		buildDir = absPath
	}

	sysrootPath := f.sysroot
	if sysrootPath != "" && !filepath.IsAbs(sysrootPath) {
		absPath, err := filepath.Abs(sysrootPath)
		if err != nil {
			logger.Errorf("resolving sysroot: %v", err)
			os.Exit(1)
		}
		sysrootPath = absPath
	}

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		logger.Errorf("creating build directory: %v", err)
		os.Exit(1)
	}

	f.builddir = buildDir
	f.sysroot = sysrootPath

	logger.SetVerbose(f.verbose)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Errorf("loading configuration: %v", err)
		os.Exit(1)
	}

	toolchainCfg, _, err := config.LoadToolchainConfig(f.toolchainFile)
	if err != nil {
		logger.Errorf("loading toolchain configuration: %v", err)
		os.Exit(1)
	}

	if toolchainCfg != nil {
		cfg.Toolchain = config.MergeToolchainConfig(&cfg.Toolchain, toolchainCfg)
	}

	if f.list {
		for _, pkg := range cfg.Packages {
			fmt.Println(pkg.Name)
		}
		os.Exit(0)
	}

	if f.sysroot == "" {
		logger.Warn("No sysroot specified. Packages will be installed to system root (/).")
		fmt.Print("This may modify your system. Continue? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			logger.Errorf("reading input: %v", err)
			os.Exit(1)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			logger.Info("Aborted.")
			os.Exit(0)
		}
	}

	archValue := f.arch
	if archValue == "" && cfg.Toolchain.Arch != "" {
		archValue = cfg.Toolchain.Arch
	}
	hostValue := f.host
	if hostValue == "" && cfg.Toolchain.Host != "" {
		hostValue = cfg.Toolchain.Host
	}

	if len(packageFilter) > 0 {
		for _, pkgName := range packageFilter {
			if cfg.GetPackageByName(pkgName) == nil {
				logger.Errorf("package '%s' not found in configuration", pkgName)
				os.Exit(1)
			}
		}
		logger.Info("Loaded %d packages from %s (filtered to %d)", len(cfg.Packages), cfg.FilePath, len(packageFilter))
	} else {
		logger.Info("Loaded %d packages from %s", len(cfg.Packages), cfg.FilePath)
	}
	if sysrootPath != "" {
		logger.Info("Using sysroot: %s", sysrootPath)
	}
	logger.Info("Concurrency: %d", f.jobs)
	logger.Info("")

	makepkgCmd, err := f.MakepkgCommand(cfg)
	if err != nil {
		logger.Errorf("building makepkg command: %v", err)
		os.Exit(1)
	}

	builderCfg := build.BuilderConfig{
		Quiet:          f.quiet,
		Verbose:        f.verbose,
		FailFast:       f.failFast,
		DryRun:         f.dryRun,
		AlwaysInstall:  f.alwaysInstall,
		MaxConcurrency: f.jobs,
		MakeJobs:       f.makeJobs,
	}

	builder, err := build.NewBuilder(builderCfg, cfg, buildDir, sysrootPath, hostValue, makepkgCmd)
	if err != nil {
		logger.Errorf("creating builder: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ctx = setupSignalHandler(ctx)
	if f.alwaysMake {
		if err := builder.Clean(packageFilter); err != nil {
			logger.Errorf("Clean process encountered errors: %v", err)
		}
		if err := builder.Build(ctx, packageFilter); err != nil {
			logger.Errorf("Build process encountered errors: %v", err)
		}

		builder.PrintSummary()
	} else if f.clean {
		if err := builder.Clean(packageFilter); err != nil {
			logger.Errorf("Clean process encountered errors: %v", err)
		}
	} else {
		if err := builder.Build(ctx, packageFilter); err != nil {
			logger.Errorf("Build process encountered errors: %v", err)
		}

		builder.PrintSummary()
	}
}

func setupSignalHandler(ctx context.Context) context.Context {
	close(signalHandler)
	ctx, cancel := context.WithCancelCause(ctx)

	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		cancel(context.Canceled)
		<-c
		os.Exit(1)
	}()

	return ctx
}
