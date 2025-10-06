package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"

	"github.com/aar10n/makepkg/pkg"
)

func main() {
	// Define command-line flags
	configFile := pflag.StringP("file", "f", "", "Read `FILE` as the package configuration file")
	toolchainFile := pflag.StringP("toolchain", "t", "", "Read `FILE` as the toolchain configuration file")
	sysroot := pflag.StringP("sysroot", "s", "", "The `PATH` to use as the sysroot when installing and building")
	builddir := pflag.StringP("builddir", "b", "build", "The `PATH` to the directory where packages should be built")
	arch := pflag.StringP("arch", "a", "", "The target `ARCH` to build for (e.g., x86_64)")
	host := pflag.StringP("host", "h", "", "The target `HOST` to build for (e.g., x86_64-linux-musl)")
	jobs := pflag.IntP("jobs", "j", 1, "The maximum concurrency `N` for building packages")
	quiet := pflag.BoolP("quiet", "q", false, "Do not log build output, only info and summary")
	failFast := pflag.BoolP("fail-fast", "F", false, "Stop building immediately on first error")
	dryRun := pflag.BoolP("dry-run", "n", false, "Print what would be done without actually building")
	verbose := pflag.BoolP("verbose", "v", false, "Enable verbose debug logging")
	clean := pflag.Bool("clean", false, "Clean package builds instead of building them")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] [package...]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "A generic build system for system packages.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArguments:\n")
		fmt.Fprintf(os.Stderr, "  package...  One or more packages to build/clean (default: all packages)\n")
	}

	pflag.Parse()

	configPath := *configFile
	packageFilter := pflag.Args()

	if configPath != "" {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: configuration file %s not found\n", configPath)
			os.Exit(1)
		}
	}

	buildDir := *builddir
	if !filepath.IsAbs(buildDir) {
		absPath, err := filepath.Abs(buildDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving build directory: %v\n", err)
			os.Exit(1)
		}
		buildDir = absPath
	}

	sysrootPath := *sysroot
	if sysrootPath != "" && !filepath.IsAbs(sysrootPath) {
		absPath, err := filepath.Abs(sysrootPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving sysroot: %v\n", err)
			os.Exit(1)
		}
		sysrootPath = absPath
	}

	if *sysroot == "" {
		fmt.Println("Warning: No sysroot specified. Packages will be installed to system root (/).")
		fmt.Print("This may modify your system. Continue? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating build directory: %v\n", err)
		os.Exit(1)
	}

	pkg.VerboseLogging = *verbose

	config, err := pkg.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	toolchainConfig, err := pkg.LoadToolchainConfig(*toolchainFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading toolchain configuration: %v\n", err)
		os.Exit(1)
	}

	if toolchainConfig != nil {
		config.Toolchain = pkg.MergeToolchainConfig(&config.Toolchain, toolchainConfig)
	}

	archValue := *arch
	if archValue == "" && config.Toolchain.Arch != "" {
		archValue = config.Toolchain.Arch
	}
	hostValue := *host
	if hostValue == "" && config.Toolchain.Host != "" {
		hostValue = config.Toolchain.Host
	}

	if len(packageFilter) > 0 {
		for _, pkgName := range packageFilter {
			if config.GetPackageByName(pkgName) == nil {
				fmt.Fprintf(os.Stderr, "Error: package '%s' not found in configuration\n", pkgName)
				os.Exit(1)
			}
		}
		fmt.Printf("Loaded %d packages from %s (filtered to %d)\n", len(config.Packages), config.ConfigPath, len(packageFilter))
	} else {
		fmt.Printf("Loaded %d packages from %s\n", len(config.Packages), config.ConfigPath)
	}
	if sysrootPath != "" {
		fmt.Printf("Sysroot: %s\n", sysrootPath)
	}
	fmt.Printf("Concurrency: %d\n", *jobs)
	fmt.Println()

	// Create builder config
	builderCfg := pkg.BuilderConfig{
		Quiet:          *quiet,
		Verbose:        *verbose,
		FailFast:       *failFast,
		DryRun:         *dryRun,
		MaxConcurrency: *jobs,
	}

	// Create builder
	builder, err := pkg.NewBuilder(config, buildDir, sysrootPath, hostValue, builderCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating builder: %v\n", err)
		os.Exit(1)
	}

	// Run clean or build
	if *clean {
		if err := builder.Clean(packageFilter); err != nil {
			fmt.Fprintf(os.Stderr, "Clean process encountered errors: %v\n", err)
		}
	} else {
		if err := builder.Build(packageFilter); err != nil {
			fmt.Fprintf(os.Stderr, "Build process encountered errors: %v\n", err)
		}

		// Print summary
		builder.PrintSummary()
	}
}
