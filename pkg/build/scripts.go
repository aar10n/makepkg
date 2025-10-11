package build

// ScriptType represents the type of script being executed.
type ScriptType string

const (
	ScriptTypeBuild   ScriptType = "build"
	ScriptTypeInstall ScriptType = "install"
	ScriptTypeClean   ScriptType = "clean"
)

const commonFunctions = `
# Common helper functions for makepkg scripts

# Print an informational message
mkpkg::info() {
	echo "[INFO] $@"
}

# Print a warning message
mkpkg::warn() {
	echo "[WARN] $@" >&2
}

# Print an error message and exit
mkpkg::error() {
	echo "[ERROR] $@" >&2
	exit 1
}

# Check if a command exists
mkpkg::has_command() {
	command -v "$1" >/dev/null 2>&1
}

# Apply a patch file
mkpkg::apply_patch() {
	local patch_file="$1"
	if [ ! -f "$patch_file" ]; then
		mkpkg::error "Patch file not found: $patch_file"
	fi
	mkpkg::info "Applying patch: $patch_file"
	patch -p1 < "$patch_file" || mkpkg::error "Failed to apply patch: $patch_file"
}

# Replace text in a file (sed wrapper)
mkpkg::replace_in_file() {
	local pattern="$1"
	local replacement="$2"
	local file="$3"
	if [ ! -f "$file" ]; then
		mkpkg::error "File not found: $file"
	fi
	sed -i.bak "s|$pattern|$replacement|g" "$file"
}
`

const buildFunctions = `
# Build-specific helper functions

# Configure a package using the standard ./configure script
mkpkg::configure() {
	if [ ! -f "./configure" ]; then
		mkpkg::error "configure script not found"
	fi

	mkpkg::info "Running configure..."
	./configure \
		--host=$PKGS_HOST \
		--prefix=/usr \
		"$@"
}
`

// installFunctions contains bash functions available to install scripts.
const installFunctions = `
# Install-specific helper functions

# Run make install with DESTDIR
mkpkg::make_install() {
	mkpkg::info "Running make install to $SYS_ROOT..."
	mkpkg::info "make install $@ DESTDIR=$SYS_ROOT"
	make install "$@" DESTDIR="$SYS_ROOT"
}

# Install a file to a specific location
#   $1 - source file path
#   $2 - destination path within SYS_ROOT
#   $3 - optional file mode (defaults to 0644)
mkpkg::install_file() {
	local src="$1"
	local dst="$2"
	local mode="${3:-0644}"

	if [ ! -f "$src" ]; then
		mkpkg::error "Source file not found: $src"
	fi

	local full_dst="$SYS_ROOT$dst"
	mkpkg::info "Installing $src to $dst"

	mkdir -p "$(dirname "$full_dst")"
	install -m "$mode" "$src" "$full_dst"
}

# Copies a file to the package build artifact directory
#   $1 - source file path
#   $2 - optional destination path within artifact dir (defaults to basename of source)
mkpkg::write_artifact() {
	if [ ! -f "$1" ]; then
		mkpkg::error "Artifact file not found: $1"
	fi

	local artifact_path
	if [ -z "$2" ]; then
		artifact_path="$BUILD_ARTIFACTS/$PKG_NAME/$(basename "$1")"
	else
		artifact_path="$BUILD_ARTIFACTS/$PKG_NAME/$2"
	fi
	
	mkpkg::info "Writing artifact to $artifact_path"
	mkdir -p "$BUILD_ARTIFACTS/$PKG_NAME"
	cp "$1" "$artifact_path"
}

# Copies a file from the specified package build artifact directory
#   $1 - target package name
#   $2 - source file path within artifact dir
#   $3 - optional destination path (defaults to basename of source)
mkpkg::get_artifact() {
	local artifact_path="$BUILD_ARTIFACTS/$1/$2"
	local dest_path
	if [ -z "$3" ]; then
		dest_path="$(basename "$2")"
	else
		dest_path="$3"
	fi
	
	if [ -f "$artifact_path" ]; then
		mkpkg::info "Getting artifact from $artifact_path"
		cp "$artifact_path" "$dest_path"
		return 0
	fi

	# artifact not found, try reinstalling the target package
	$MAKEPKG -I $1 > /dev/null 2>&1

	# try again
	if [ ! -f "$artifact_path" ]; then
		mkpkg::error "Artifact file not found: $artifact_path"
	fi
	
	mkpkg::info "Getting artifact from $artifact_path"
	cp "$artifact_path" "$dest_path"
}
`

// GetScriptPreamble returns the bash functions to prepend to a script.
func GetScriptPreamble(scriptType ScriptType) string {
	preamble := "#!/bin/bash\nset -e\n\n"
	preamble += commonFunctions + "\n"

	switch scriptType {
	case ScriptTypeBuild:
		preamble += buildFunctions + "\n"
	case ScriptTypeInstall:
		preamble += installFunctions + "\n"
	case ScriptTypeClean:
		// Clean scripts only get common functions
	default:
		// Default to common only
	}

	return preamble
}
