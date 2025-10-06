# makepkg

A build system designed for building and installing third-party packages into custom sysroots (e.g., for creating initrd images).

## Features

- **Declarative YAML configuration** - Define packages with source URLs, build commands, and dependencies
- **Dependency resolution** - Automatically builds packages in the correct order based on dependencies
- **Concurrent builds** - Build independent packages in parallel with configurable concurrency
- **Smart caching** - Only rebuilds packages when source URL or build/install commands change
- **Cross-compilation support** - Specify toolchain prefix for cross-compilation
- **Sysroot isolation** - Install packages to a custom sysroot without affecting the host system
- **Multiple archive formats** - Supports tar.gz, tar.bz2, tar.xz archives

## Installation

```bash
go build -o makepkg
```

Or install directly:

```bash
go install github.com/aar10n/makepkg@latest
```

## Usage

```bash
makepkg [OPTIONS] [packages.yaml]
```

### Options

- `--sysroot PATH` - The path to use as the sysroot when installing and building (packages in sysroot are locatable for linking)
- `--builddir PATH` - The path to the directory where packages should be built (default: `build`)
- `-j, --jobs N` - Maximum concurrency for building packages (default: 1, sequential)
- `-q, --quiet` - Do not log build output, only program info logs and summary

### Package Configuration

Create a `packages.yaml` file with the following structure:

```yaml
packages:
  - name: zlib
    url: https://zlib.net/zlib-1.3.1.tar.gz
    build: |
      ./configure --prefix=/usr --static
      make
    install: |
      make install DESTDIR=$INSTALL_ROOT

  - name: curl
    url: https://curl.se/download/curl-8.5.0.tar.gz
    build: |
      ./configure --prefix=/usr --enable-static --disable-shared --with-zlib
      make
    install: |
      install -D -m 755 src/curl $INSTALL_ROOT/usr/bin/curl
    depends_on:
      - zlib
```

### Environment Variables

During the build and install phases, the following environment variables are automatically set:

**Build phase:**
- `CC`, `CXX`, `AR`, `AS`, `LD`, etc. (if `--toolchain` is specified)
- `PKG_CONFIG_PATH`, `PKG_CONFIG_SYSROOT_DIR` (if `--sysroot` is specified)
- `CFLAGS`, `CXXFLAGS`, `LDFLAGS` (with appropriate include/library paths from sysroot)
- `LIBRARY_PATH`, `LD_LIBRARY_PATH` (pointing to sysroot libraries)

**Install phase:**
- All build environment variables, plus:
- `INSTALL_ROOT` - The path to the sysroot (or `/` if no sysroot specified)

## Examples

### Basic usage with sysroot

```bash
# Build all packages and install to ./sysroot
makepkg --sysroot ./sysroot packages.yaml
```

### Cross-compilation with concurrency

```bash
# Cross-compile for ARM with 4 parallel jobs
makepkg --sysroot ./initrd \
           --toolchain arm-linux-gnueabi- \
           --jobs 4 \
           packages.yaml
```

### Quiet mode

```bash
# Build without showing build output
makepkg --sysroot ./sysroot --quiet packages.yaml
```

## How it Works

1. **Configuration Loading** - Parses the YAML package definitions
2. **Dependency Resolution** - Uses topological sorting to determine build order
3. **Cache Check** - Compares current package definition with cached version
4. **Download & Extract** - Downloads source archives and extracts to `<builddir>/<package>/source`
5. **Build** - Executes build commands in the source directory with appropriate environment
6. **Install** - Executes install commands with `INSTALL_ROOT` set
7. **Cache Update** - Saves package URL and config for future builds
8. **Summary** - Displays build results for all packages

## Build Directory Structure

```
build/
├── zlib/
│   ├── url              # Cached source URL
│   ├── config           # Cached build/install commands
│   ├── zlib-1.3.1.tar.gz
│   └── source/          # Extracted source code
└── curl/
    ├── url
    ├── config
    ├── curl-8.5.0.tar.gz
    └── source/
```

## License

MIT
