MAKEPKG(1)                  General Commands Manual                 MAKEPKG(1)

NAME
     makepkg - a generic build system for system packages

SYNOPSIS
     makepkg [-f file] [-t file] [-s path] [-b path] [-a arch] [-h host]
             [-j N] [-m N] [-qFnvBI] [--clean] [--list] [--version]
             [package ...]

DESCRIPTION
     The makepkg utility is a package build system that automates the process
     of downloading, building, and installing software packages with support
     for dependency resolution, cross-compilation toolchains, and concurrent
     builds.

     makepkg reads package definitions from a YAML or TOML configuration file,
     resolves dependencies, and builds packages in the correct order.  Each
     package can specify build scripts, installation scripts, and dependencies
     on other packages.

     If no configuration file is specified, makepkg searches for
     packages.yaml, packages.yml, or packages.toml in the current directory
     (in that order).

OPTIONS
     The following options are available:

     -f file, --file file
             Read file as the package configuration file.  If not specified,
             makepkg auto-discovers a configuration file in the current
             directory.

     -t file, --toolchain file
             Read file as the toolchain configuration file.  If not specified,
             makepkg attempts to auto-discover toolchain.yaml, toolchain.yml,
             or toolchain.toml in the current directory.  The toolchain
             configuration may also be embedded in the packages configuration
             file.

     -s path, --sysroot path
             Use path as the sysroot directory when installing and building
             packages.  If not specified, packages will be installed to the
             system root (/), after prompting for confirmation.  The sysroot
             path is made absolute and exported as the SYS_ROOT environment
             variable.

     -b path, --builddir path
             Use path as the directory where packages should be built.
             Defaults to build in the current directory.  Each package creates
             a subdirectory within the build directory.

     -a arch, --arch arch
             Set the target architecture to arch (e.g., x86_64, aarch64, arm).
             This overrides the architecture specified in the toolchain
             configuration.  The value is exported as PKGS_ARCH.

     -h host, --host host
             Set the target host triple to host (e.g., x86_64-linux-musl, arm-
             linux-gnueabihf).  This overrides the host specified in the
             toolchain configuration.  The value is exported as PKGS_HOST.

     -j N, --jobs N
             Set the maximum concurrency to N for building packages.  Defaults
             to 1 (sequential builds).  Packages at the same dependency level
             are built concurrently up to this limit.

     -m N, --make-jobs N
             Set the number of jobs to N for each make invocation.  Defaults
             to 1.  This value is exported as MAKEFLAGS in the format `-jN'.

     -q, --quiet
             Do not log build output to standard output.  Only informational
             messages and the build summary are displayed.  Build output is
             still captured for error reporting.

     -F, --fail-fast
             Stop building immediately when the first error occurs.  By
             default, makepkg continues building other packages after a
             failure.

     -n, --dry-run
             Print what would be done without actually building packages.
             Shows which packages would be downloaded, built, or skipped based
             on cache state.

     -v, --verbose
             Enable verbose debug logging.  Shows detailed information about
             environment variable substitution, toolchain configuration, and
             build environment setup.

     -B, --always-make
             Clean then build packages, forcing a complete rebuild.  This is
             equivalent to running makepkg with --clean followed by a normal
             build.

     -I, --always-install
             Always reinstall packages, ignoring cache state.  Even if a
             package is up to date, it will be reinstalled to the sysroot.

     --clean
             Clean package builds instead of building them.  For each package,
             makepkg attempts to run the package's custom clean script if
             specified, falls back to `make clean', or removes the source
             directory entirely.

     --list  List all package names from the configuration file and exit.

     -V, --version
             Show version information and exit.

ARGUMENTS
     If one or more package arguments are specified, only those packages (and
     their dependencies) are built.  All package names must be defined in the
     configuration file.  If no packages are specified, all packages defined
     in the configuration are built.

PACKAGE CONFIGURATION FORMAT
     The package configuration file may be written in YAML or TOML format.
     The format is determined by the file extension (.yaml, .yml, .toml) or by
     attempting to parse the file as both formats.

     The configuration file contains two top-level sections:

     toolchain
             Optional toolchain configuration (see TOOLCHAIN CONFIGURATION).
             May be omitted if toolchain settings are provided in a separate
             file.

     packages
             An array of package definitions.  Each package must contain the
             following fields:
             name        Unique package identifier
             url         URL to download the package source archive
             build       Shell script to compile the package
             install     Shell script to install the package

             Optional package fields:
             native      Boolean flag indicating if this is a native build.
                         When true, toolchain environment variables are not
                         added to the build environment.  Defaults to false.
             clean       Custom shell script for cleaning the package
             env         Array of environment variables in `NAME=VALUE'
                         format. May reference other make
             depends_on  Array of package names this package depends on

     Example YAML configuration:

           toolchain:
             arch: x86_64
             host: x86_64-linux-musl
             bin: /opt/cross/bin
             cross_prefix: x86_64-linux-musl-

           packages:
             - name: zlib
               url: https://zlib.net/zlib-1.3.tar.gz
               build: |
                 mkpkg::configure
                 make
               install: |
                 mkpkg::make_install

             - name: build-tools
               url: https://example.com/tools-1.0.tar.gz
               native: true
               build: |
                 # Native build - toolchain vars not set
                 ./configure --prefix=/usr
                 make
               install: |
                 mkpkg::make_install
                 mkpkg::write_artifact mytool

             - name: openssl
               url: https://www.openssl.org/source/openssl-3.0.0.tar.gz
               depends_on:
                 - zlib
                 - build-tools
               env:
                 - CFLAGS=-O2 -g
               build: |
                 mkpkg::get_artifact build-tools mytool
                 ./config --prefix=/usr --openssldir=/etc/ssl
                 make
               install: |
                 mkpkg::make_install
               clean: |
                 make distclean


TOOLCHAIN CONFIGURATION
     Toolchain settings may be specified in the packages configuration file
     under the toolchain section, or in a separate file (specified via -t or
     auto-discovered as toolchain.yaml, toolchain.yml, or toolchain.toml).

     A separate toolchain file takes precedence over the toolchain section in
     the packages configuration.  Command-line flags -a and -h override both
     configuration sources.

     Toolchain configuration fields:

     arch          Target architecture (e.g., x86_64, aarch64, arm).  Exported
                   as PKGS_ARCH.

     host          Target host triple (e.g., x86_64-linux-musl).  Exported as
                   PKGS_HOST.

     bin           Directory containing toolchain binaries.  This directory is
                   prepended to PATH.

     cross_prefix  Prefix for cross-compilation tools (e.g., x86_64-linux-
                   musl-).  Combined with bin to create full paths to
                   toolchain programs.  Exported as CROSS_PREFIX.

     extra_programs
                   Array of additional program names to export.  These are
                   combined with bin to create environment variables.

     For each standard toolchain program (ar, as, ld, nm, objcopy, objdump,
     ranlib, strip, addr2line, c++filt, dlltool, elfedit, gprof, readelf,
     size, strings, gcc, g++), an environment variable is created by combining
     bin and cross_prefix with the program name.  The environment variable
     name is the program name in uppercase, with hyphens converted to
     underscores and plus signs converted to X.

     For example, with bin set to /opt/cross/bin and cross_prefix set to
     `x86_64-linux-musl-', the following environment variables are created:

           CC=/opt/cross/bin/x86_64-linux-musl-gcc
           CXX=/opt/cross/bin/x86_64-linux-musl-g++
           AR=/opt/cross/bin/x86_64-linux-musl-ar
           LD=/opt/cross/bin/x86_64-linux-musl-ld

     Example standalone toolchain.yaml:

           arch: aarch64
           host: aarch64-linux-gnu
           bin: /usr/local/cross/bin
           cross_prefix: aarch64-linux-gnu-

ENVIRONMENT VARIABLE SUBSTITUTION
     makepkg supports environment variable substitution using the ${VAR}
     syntax in most package and toolchain configuration fields.  This is
     similar to shell variable expansion but uses a dedicated substitution
     engine separate from the shell scripts.  Variables are expanded using
     envsubst(1)-like syntax.

     The following fields support variable substitution:
     o   Package url field
     o   Package build, install, and clean scripts
     o   Package env values (the part after the equals sign)
     o   Toolchain arch, host, bin, and cross_prefix
     o   Toolchain extra_programs entries

     During package configuration substitution, the following special
     variables are automatically provided:

     PKG_NAME  The name of the current package being processed.

     PKG_URL   The URL of the current package (before substitution).

     FILE_DIR  The absolute path to the directory containing the package
               configuration file.

     During toolchain configuration substitution:

     FILE_DIR  The absolute path to the directory containing the toolchain
               configuration file.

     Variables are expanded before scripts are executed and before the build
     environment is constructed.  Undefined variables in toolchain
     configuration fields cause an error.  Undefined variables in package
     scripts are left unexpanded as literal text.

     Note that substitution syntax ${VAR} is distinct from regular shell
     variable syntax $VAR or ${VAR}.  In package scripts, both substitution
     forms may appear:

     o   ${VAR} substitution happens first, during configuration loading,
         using makepkg's internal substitution engine

     o   $VAR and ${VAR} are then processed by the shell when scripts execute

     This means ${PKGS_HOST} in a build script will be substituted by makepkg
     before the script runs, while $(nproc) or $HOME will be expanded by the
     shell during script execution.

ENVIRONMENT VARIABLES
     The following environment variables are set by makepkg and made available
     to package build and install scripts:

   Core Variables
     PKGS_ROOT        Absolute path to the directory containing the package
                      configuration file.

     PKGS_ARCH        Target architecture (from toolchain configuration or -a
                      flag).

     PKGS_HOST        Target host triple (from toolchain configuration or -h
                      flag).

     BUILD_DIR        Absolute path to the build directory.

     SYS_ROOT         Absolute path to the sysroot directory (if specified via
                      -s).  This variable is always set, defaulting to / if no
                      sysroot is specified.

     INSTALL_ROOT     Alias for SYS_ROOT.  Provided for compatibility with
                      packages that expect this variable name.

     MAKEPKG          The full command line to invoke makepkg with the same
                      configuration.  Used by helper functions like
                      mkpkg::get_artifact() to rebuild dependency packages
                      when needed.

     BUILD_ARTIFACTS  Absolute path to the build artifacts directory at
                      $BUILD_DIR/artifacts.  Each package has its own
                      subdirectory at $BUILD_ARTIFACTS/$PKG_NAME for storing
                      inter-package build artifacts.  Package-specific
                      artifact directories are cleaned before each build.

     MAKEFLAGS        Set to `-jN' where N is the value of the -m flag.
                      Controls parallelism for make-based builds.

   Toolchain Variables
     CROSS_PREFIX  The cross-compilation prefix (if specified in toolchain
                   configuration).

     CC, CXX, AR, LD, AS, NM, RANLIB, STRIP
                   Paths to the corresponding toolchain programs.  Variable
                   names follow the pattern described in TOOLCHAIN
                   CONFIGURATION.

   Sysroot Variables
     When a sysroot is specified, the following variables are configured:

     PKG_CONFIG_PATH         Prepended with ${SYS_ROOT}/usr/lib/pkgconfig to
                             ensure pkg-config finds libraries installed in
                             the sysroot.

     PKG_CONFIG_SYSROOT_DIR  Set to SYS_ROOT.

     CFLAGS, CXXFLAGS        Prepended with -I${SYS_ROOT}/usr/include.

     LDFLAGS                 Prepended with -L${SYS_ROOT}/usr/lib and
                             -L${SYS_ROOT}/lib.

     LIBRARY_PATH, LD_LIBRARY_PATH
                             Include ${SYS_ROOT}/usr/lib and ${SYS_ROOT}/lib.

     Environment variables specified in a package's env field are added to the
     build and install environment.  Values in env undergo variable
     substitution before being set.

PACKAGE SCRIPT FUNCTIONS
     makepkg provides a set of bash helper functions to package scripts to
     simplify common tasks.  These functions are automatically available in
     all build, install, and clean scripts.

   Common Functions
     The following functions are available in all script types:

     mkpkg::info(message...)
                             Print an informational message to standard
                             output.

     mkpkg::warn(message...)
                             Print a warning message to standard error.

     mkpkg::error(message...)
                             Print an error message to standard error and exit
                             with status 1.

     mkpkg::has_command(command)
                             Check if a command exists in PATH.  Returns 0 if
                             the command exists, non-zero otherwise.

     mkpkg::apply_patch(patch_file)
                             Apply a patch file using patch(1) with -p1.
                             Exits with an error if the patch file is not
                             found or fails to apply.

     mkpkg::replace_in_file(pattern, replacement, file)
                             Replace all occurrences of a pattern with a
                             replacement in a file using sed(1).  Creates a
                             backup file with .bak extension.  Exits with an
                             error if the file is not found.

   Build Script Functions
     The following functions are available only in build scripts:

     mkpkg::configure(args...)
                       Run a standard ./configure script with common cross-
                       compilation arguments.  Automatically passes
                       --host=$PKGS_HOST and --prefix=/usr, along with any
                       additional arguments provided.  Exits with an error if
                       the configure script is not found.

   Install Script Functions
     The following functions are available only in install scripts:

     mkpkg::make_install(args...)
                          Run `make install' with `DESTDIR=$SYS_ROOT' and any
                          additional arguments.  This is the recommended way
                          to install packages.

     mkpkg::install_file(source, dest, mode)
                          Install a single file to a specific location within
                          the sysroot.  The source is the path to the file in
                          the build directory.  The dest is the absolute path
                          within the sysroot (e.g., /usr/bin/program).  The
                          optional mode is the file permission mode (defaults
                          to 0644).  Creates parent directories as needed.
                          Exits with an error if the source file is not found.

     mkpkg::write_artifact(source, dest)
                          Copy a file to the current package's build artifact
                          directory for use by other packages.  The source is
                          the path to the file in the build directory.  The
                          optional dest is the destination path within the
                          artifact directory (defaults to the basename of the
                          source file).  Artifacts are stored at
                          $BUILD_ARTIFACTS/$PKG_NAME/.  Package artifact
                          directories are cleaned before each build.

     mkpkg::get_artifact(package, source, dest)
                          Copy a file from another package's build artifact
                          directory.  The package is the name of the package
                          that created the artifact.  The source is the path
                          within that package's artifact directory.  The
                          optional dest is the destination path (defaults to
                          the basename of the source).  Exits with an error if
                          the artifact still cannot be found after rebuilding.

     Example usage in install script:

           install: |
             mkpkg::make_install
             mkpkg::install_file config.h /usr/include/mylib/ 0644
             mkpkg::write_artifact libmylib.a

     Example using artifacts from another package:

           install: |
             mkpkg::get_artifact mylib libmylib.a
             mkpkg::make_install
           depends_on:
             - mylib

CACHING AND REBUILDING
     makepkg maintains cache metadata in the build directory to avoid
     unnecessary rebuilds.  For each package, a cache file makepkg.json is
     stored in the package's build directory subdirectory.

     A package is rebuilt if:

     o   The package has never been built

     o   The package URL has changed

     o   The build script has changed

     o   The install script has changed

     o   The target host has changed

     o   Dependencies have changed

     A package is reinstalled (without rebuilding) if:

     o   The build is up-to-date but the sysroot path has changed

     Cache state is displayed in dry-run mode (-n) and when verbose logging is
     enabled (-v).

DEPENDENCY RESOLUTION
     makepkg uses topological sorting to determine the build order of packages
     based on their depends_on declarations.  Packages are built in levels,
     where all packages in a level have no dependencies on packages in later
     levels.

     Within a single dependency level, packages may be built concurrently
     (controlled by the -j flag).

     Circular dependencies are detected and reported as errors before any
     builds begin.

EXIT STATUS
     The makepkg utility exits 0 on success, and >0 if an error occurs.  The
     makepkg utility exits with a non-zero status if any package fails to
     build, unless errors are ignored (non-fail-fast mode).

EXAMPLES
     Build all packages using auto-discovered configuration:

           $ makepkg -s /tmp/sysroot -j 4

     Build only the `openssl' package and its dependencies:

           $ makepkg -s /tmp/sysroot openssl

     Perform a dry run to see what would be built:

           $ makepkg -n -s /tmp/sysroot

     Build with a specific toolchain configuration:

           $ makepkg -f packages.yaml -t arm-toolchain.yaml \
               -s /opt/sysroot -h arm-linux-gnueabihf

     Clean all built packages:

           $ makepkg --clean

     Build with verbose logging for debugging:

           $ makepkg -v -s /tmp/sysroot

SEE ALSO
     bash(1), make(1), pkg-config(1)

AUTHORS
     Aaron Gill-Braun

BUGS
     Report bugs at: https://github.com/aar10n/makepkg

macOS 15.5                      January 5, 2025                     macOS 15.5
