package pkg

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

func isGitURL(url string) bool {
	return strings.HasSuffix(url, ".git")
}

// cloneGitRepo clones a git repository with depth=1 to the source directory
func cloneGitRepo(sourceDir, url string) error {
	cmd := exec.Command("git", "clone", "--depth=1", url, sourceDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// DownloadAndExtract downloads a source archive and extracts it to the source directory
// or clones a git repository if the URL is a git URL
func DownloadAndExtract(buildDir, pkgName, url string) error {
	pkgDir := filepath.Join(buildDir, pkgName)
	sourceDir := filepath.Join(pkgDir, "source")

	// Create package directory
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	// Check if this is a git URL
	if isGitURL(url) {
		return cloneGitRepo(sourceDir, url)
	}

	// Determine archive filename from URL
	urlParts := strings.Split(url, "/")
	filename := urlParts[len(urlParts)-1]
	archivePath := filepath.Join(pkgDir, filename)

	// Download the file
	if err := downloadFile(archivePath, url); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Create source directory
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return fmt.Errorf("failed to create source directory: %w", err)
	}

	// Extract the archive
	if err := extractArchive(archivePath, sourceDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return nil
}

// downloadFile downloads a file from a URL to a local path
func downloadFile(path, url string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// extractArchive extracts a tar archive (with optional compression) to a target directory
func extractArchive(archivePath, targetDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file

	// Determine compression type from file extension
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tgz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	} else if strings.HasSuffix(archivePath, ".bz2") {
		reader = bzip2.NewReader(file)
	} else if strings.HasSuffix(archivePath, ".xz") {
		xzReader, err := xz.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create xz reader: %w", err)
		}
		reader = xzReader
	}

	tarReader := tar.NewReader(reader)

	// Track the top-level directory to strip it
	var topLevelDir string
	firstEntry := true

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		// Skip PAX global headers when detecting top-level directory
		if header.Typeflag == tar.TypeXGlobalHeader {
			debugLog("Skipping PAX global header")
			continue
		}

		// Detect top-level directory from first real entry
		if firstEntry {
			parts := strings.Split(header.Name, "/")
			if len(parts) > 0 {
				topLevelDir = parts[0]
			}
			firstEntry = false
			debugLog("Detected top-level directory: %s (from: %s)", topLevelDir, header.Name)
		}

		// Strip the top-level directory
		name := header.Name
		if topLevelDir != "" && strings.HasPrefix(name, topLevelDir+"/") {
			name = strings.TrimPrefix(name, topLevelDir+"/")
			debugLog("Stripped prefix from %s -> %s", header.Name, name)
		} else if name == topLevelDir {
			debugLog("Skipping top-level directory: %s", name)
			continue
		}

		if name == "" {
			debugLog("Skipping empty name (was: %s)", header.Name)
			continue
		}

		target := filepath.Join(targetDir, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()

		case tar.TypeSymlink:
			_ = os.Symlink(header.Linkname, target)
		}
	}

	return nil
}
