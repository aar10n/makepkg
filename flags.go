package main

import (
	"fmt"
	"github.com/aar10n/makepkg/pkg/config"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

// flags holds all command-line flag values
type flags struct {
	configFile    string
	toolchainFile string
	sysroot       string
	builddir      string
	arch          string
	host          string
	jobs          int
	makeJobs      int
	quiet         bool
	failFast      bool
	dryRun        bool
	verbose       bool
	list          bool
	clean         bool
	alwaysMake    bool
	alwaysInstall bool
	showVersion   bool
}

func parseFlags() *flags {
	f := &flags{}

	pflag.StringVarP(&f.configFile, "file", "f", "", "Read `FILE` as the package configuration file")
	pflag.StringVarP(&f.toolchainFile, "toolchain", "t", "", "Read `FILE` as the toolchain configuration file")
	pflag.StringVarP(&f.sysroot, "sysroot", "s", "", "The `PATH` to use as the sysroot when installing and building")
	pflag.StringVarP(&f.builddir, "builddir", "b", "build", "The `PATH` to the directory where packages should be built")
	pflag.StringVarP(&f.arch, "arch", "a", "", "The target `ARCH` to build for (e.g., x86_64)")
	pflag.StringVarP(&f.host, "host", "h", "", "The target `HOST` to build for (e.g., x86_64-linux-musl)")
	pflag.IntVarP(&f.jobs, "jobs", "j", 1, "The maximum concurrency `N` for building packages")
	pflag.IntVarP(&f.makeJobs, "make-jobs", "m", 1, "The number of jobs `N` for each make invocation")
	pflag.BoolVarP(&f.quiet, "quiet", "q", false, "Do not log build output, only info and summary")
	pflag.BoolVarP(&f.failFast, "fail-fast", "F", false, "Stop building immediately on first error")
	pflag.BoolVarP(&f.dryRun, "dry-run", "n", false, "Print what would be done without actually building")
	pflag.BoolVarP(&f.verbose, "verbose", "v", false, "Enable verbose debug logging")
	pflag.BoolVar(&f.list, "list", false, "List all package names from the configuration")
	pflag.BoolVar(&f.clean, "clean", false, "Clean package builds instead of building them")
	pflag.BoolVarP(&f.alwaysMake, "always-make", "B", false, "Clean then build packages (force rebuild)")
	pflag.BoolVarP(&f.alwaysInstall, "always-install", "I", false, "Always reinstall packages ignoring cache")
	pflag.BoolVarP(&f.showVersion, "version", "V", false, "Show version information")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] [package...]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "A generic build system for system packages.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArguments:\n")
		fmt.Fprintf(os.Stderr, "  package...  One or more packages to build/clean (default: all packages)\n")
	}

	pflag.Parse()

	return f
}

func (f *flags) MakepkgCommand(cfg *config.Config) (string, error) {
	// Get the absolute path to the makepkg executable
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	var parts []string
	parts = append(parts, exePath)

	parts = append(parts, fmt.Sprintf("--file=%s", cfg.FilePath))
	parts = append(parts, fmt.Sprintf("--toolchain=%s", cfg.Toolchain.FilePath))

	if f.sysroot != "" {
		parts = append(parts, fmt.Sprintf("--sysroot=%s", f.sysroot))
	}

	if f.builddir != "" {
		parts = append(parts, fmt.Sprintf("--builddir=%s", f.builddir))
	}

	if f.arch != "" {
		parts = append(parts, fmt.Sprintf("--arch=%s", f.arch))
	}

	if f.host != "" {
		parts = append(parts, fmt.Sprintf("--host=%s", f.host))
	}

	if f.jobs > 1 {
		parts = append(parts, fmt.Sprintf("--jobs=%d", f.jobs))
	}

	if f.makeJobs > 1 {
		parts = append(parts, fmt.Sprintf("--make-jobs=%d", f.makeJobs))
	}

	if f.quiet {
		parts = append(parts, "--quiet")
	}

	if f.failFast {
		parts = append(parts, "--fail-fast")
	}

	if f.verbose {
		parts = append(parts, "--verbose")
	}

	// Note: We intentionally exclude:
	//   package targets
	//   --dry-run
	//   --always-make
	//   --always-install
	//   --clean
	return strings.Join(parts, " "), nil
}
