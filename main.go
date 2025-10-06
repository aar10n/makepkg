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
	configFile := pflag.StringP("file", "f", "", "Read `FILE` as the package configuration file")
	toolchainFile := pflag.StringP("toolchain", "t", "", "Read `FILE` as the toolchain configuration file")
	sysroot := pflag.StringP("sysroot", "s", "", "The `PATH` to use as the sysroot when installing and building")
	builddir := pflag.StringP("builddir", "b", "build", "The `PATH` to the directory where packages should be built")
	arch := pflag.StringP("arch", "a", "", "The target `ARCH` to build for (e.g., x86_64)")
	host := pflag.StringP("host", "h", "", "The target `HOST` to build for (e.g., x86_64-linux-musl)")
	jobs := pflag.IntP("jobs", "j", 1, "The maximum concurrency `N` for building packages")
	makeJobs := pflag.IntP("make-jobs", "m", 1, "The number of jobs `N` for each make invocation")
	quiet := pflag.BoolP("quiet", "q", false, "Do not log build output, only info and summary")
	failFast := pflag.BoolP("fail-fast", "F", false, "Stop building immediately on first error")
	dryRun := pflag.BoolP("dry-run", "n", false, "Print what would be done without actually building")
	verbose := pflag.BoolP("verbose", "v", false, "Enable verbose debug logging")
	clean := pflag.Bool("clean", false, "Clean package builds instead of building them")
	alwaysMake := pflag.BoolP("always-make", "B", false, "Clean then build packages (force rebuild)")
	alwaysInstall := pflag.BoolP("always-install", "I", false, "Always reinstall packages ignoring cache")
	showVersion := pflag.BoolP("version", "V", false, "Show version information")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] [package...]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "A generic build system for system packages.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArguments:\n")
		fmt.Fprintf(os.Stderr, "  package...  One or more packages to build/clean (default: all packages)\n")
	}

	pflag.Parse()

	if *showVersion {
		fmt.Printf("makepkg %s\n", version)
		if commit != "unknown" {
			fmt.Printf("commit: %s\n", commit)
		}
		if date != "unknown" {
			fmt.Printf("built: %s\n", date)
		}
		os.Exit(0)
	}

	configPath := *configFile
	packageFilter := pflag.Args()

	if configPath != "" {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			logger.Errorf("configuration file %s not found", configPath)
			os.Exit(1)
		}
	}

	buildDir := *builddir
	if !filepath.IsAbs(buildDir) {
		absPath, err := filepath.Abs(buildDir)
		if err != nil {
			logger.Errorf("resolving build directory: %v", err)
			os.Exit(1)
		}
		buildDir = absPath
	}

	sysrootPath := *sysroot
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

	logger.SetVerbose(*verbose)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Errorf("loading configuration: %v", err)
		os.Exit(1)
	}

	toolchainCfg, _, err := config.LoadToolchainConfig(*toolchainFile)
	if err != nil {
		logger.Errorf("loading toolchain configuration: %v", err)
		os.Exit(1)
	}

	if toolchainCfg != nil {
		cfg.Toolchain = config.MergeToolchainConfig(&cfg.Toolchain, toolchainCfg)
	}

	if *sysroot == "" {
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

	archValue := *arch
	if archValue == "" && cfg.Toolchain.Arch != "" {
		archValue = cfg.Toolchain.Arch
	}
	hostValue := *host
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
	logger.Info("Concurrency: %d", *jobs)
	logger.Info("")

	builderCfg := build.BuilderConfig{
		Quiet:          *quiet,
		Verbose:        *verbose,
		FailFast:       *failFast,
		DryRun:         *dryRun,
		AlwaysInstall:  *alwaysInstall,
		MaxConcurrency: *jobs,
		MakeJobs:       *makeJobs,
	}

	builder, err := build.NewBuilder(builderCfg, cfg, buildDir, sysrootPath, hostValue)
	if err != nil {
		logger.Errorf("creating builder: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ctx = setupSignalHandler(ctx)
	if *alwaysMake {
		if err := builder.Clean(packageFilter); err != nil {
			logger.Errorf("Clean process encountered errors: %v", err)
		}
		if err := builder.Build(ctx, packageFilter); err != nil {
			logger.Errorf("Build process encountered errors: %v", err)
		}

		builder.PrintSummary()
	} else if *clean {
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
