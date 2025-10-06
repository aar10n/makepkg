package download

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/aar10n/makepkg/pkg/logger"
)

const (
	maxRetries     = 3
	retryDelay     = time.Second
	requestTimeout = 5 * time.Minute
)

// Downloader defines the interface for downloading and extracting packages.
type Downloader interface {
	Download(ctx context.Context, pkgName, pkgUrl string) error
	Extract(pkgName, pkgUrl string) error
}

type downloader struct {
	buildDir string
}

var _ Downloader = (*downloader)(nil)

func NewDownloader(buildDir string) Downloader {
	return &downloader{buildDir}
}

func (d *downloader) Download(ctx context.Context, pkgName, pkgUrl string) error {
	pkgDir := filepath.Join(d.buildDir, pkgName)
	archiveFile := filepath.Join(pkgDir, getFilenameFromURL(pkgUrl))

	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	if _, err := os.Stat(archiveFile); err == nil {
		logger.Debug("File already exists at %s, skipping download", archiveFile)
		return nil
	}

	if isGitURL(pkgUrl) {
		sourceDir := filepath.Join(pkgDir, "source")
		if err := os.MkdirAll(sourceDir, 0755); err != nil {
			return fmt.Errorf("failed to create source directory: %w", err)
		}
		return cloneGitRepo(sourceDir, pkgUrl)
	}

	return downloadFile(ctx, archiveFile, pkgUrl)
}

func (d *downloader) Extract(pkgName, pkgUrl string) error {
	pkgDir := filepath.Join(d.buildDir, pkgName)
	sourceDir := filepath.Join(pkgDir, "source")
	archiveFile := filepath.Join(pkgDir, getFilenameFromURL(pkgUrl))

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return fmt.Errorf("failed to create source directory: %w", err)
	}

	if err := extractArchive(archiveFile, sourceDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return nil
}

func downloadFile(ctx context.Context, path, url string) error {
	if _, err := os.Stat(path); err == nil {
		logger.Debug("File already exists at %s, skipping download", path)
		return nil
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			delay := retryDelay * time.Duration(1<<uint(attempt-2))
			logger.Debug("Retry attempt %d/%d after %v delay", attempt, maxRetries, delay)
			time.Sleep(delay)
		}

		if err := attemptDownload(ctx, path, url); err != nil {
			lastErr = err
			logger.Warn("Download attempt %d/%d failed: %v", attempt, maxRetries, err)
			continue
		}
		return nil
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func getFilenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func isGitURL(url string) bool {
	return strings.HasSuffix(url, ".git")
}

func cloneGitRepo(sourceDir, url string) error {
	cmd := exec.Command("git", "clone", "--depth=1", url, sourceDir)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(cmdOutput))
	}
	return nil
}

func attemptDownload(ctx context.Context, path, url string) error {
	client := &http.Client{
		Timeout: requestTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
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
	if err != nil {
		os.Remove(path)
		return err
	}

	return nil
}

func extractArchive(archivePath, targetDir string) error {
	if strings.HasSuffix(archivePath, ".deb") {
		return extractDeb(archivePath, targetDir)
	} else if strings.HasSuffix(archivePath, ".snap") {
		return extractSnap(archivePath, targetDir)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file

	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tgz") || strings.HasSuffix(archivePath, ".apk") {
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
	} else if strings.HasSuffix(archivePath, ".zst") || strings.HasSuffix(archivePath, ".zstd") {
		zstdReader, err := zstd.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create zstd reader: %w", err)
		}
		defer zstdReader.Close()
		reader = zstdReader
	}

	tarReader := tar.NewReader(reader)

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

		if header.Typeflag == tar.TypeXGlobalHeader {
			logger.Debug("Skipping PAX global header")
			continue
		}

		if firstEntry {
			parts := strings.Split(header.Name, "/")
			if len(parts) > 0 {
				topLevelDir = parts[0]
			}
			firstEntry = false
			logger.Debug("Detected top-level directory: %s (from: %s)", topLevelDir, header.Name)
		}

		name := header.Name
		if topLevelDir != "" && strings.HasPrefix(name, topLevelDir+"/") {
			name = strings.TrimPrefix(name, topLevelDir+"/")
			logger.Debug("Stripped prefix from %s -> %s", header.Name, name)
		} else if name == topLevelDir {
			logger.Debug("Skipping top-level directory: %s", name)
			continue
		}

		if name == "" {
			logger.Debug("Skipping empty name (was: %s)", header.Name)
			continue
		}

		target := filepath.Join(targetDir, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
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

func extractDeb(archivePath, targetDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 8)
	if _, err := file.Read(buf); err != nil {
		return fmt.Errorf("failed to read AR magic: %w", err)
	}
	if string(buf) != "!<arch>\n" {
		return fmt.Errorf("not a valid AR archive")
	}

	for {
		header := make([]byte, 60)
		n, err := file.Read(header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read AR header: %w", err)
		}
		if n != 60 {
			break
		}

		name := strings.TrimSpace(string(header[0:16]))
		sizeStr := strings.TrimSpace(string(header[48:58]))
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse file size: %w", err)
		}

		if strings.HasPrefix(name, "data.tar") {
			logger.Debug("Found data archive in .deb: %s", name)

			data := make([]byte, size)
			if _, err := io.ReadFull(file, data); err != nil {
				return fmt.Errorf("failed to read data archive: %w", err)
			}

			return extractTarFromBytes(data, name, targetDir)
		}

		if _, err := file.Seek(size, io.SeekCurrent); err != nil {
			return fmt.Errorf("failed to skip file: %w", err)
		}

		if size%2 != 0 {
			file.Seek(1, io.SeekCurrent)
		}
	}

	return fmt.Errorf("data.tar.* not found in .deb archive")
}

func extractTarFromBytes(data []byte, name, targetDir string) error {
	reader := bytes.NewReader(data)
	var tarReader io.Reader = reader

	if strings.HasSuffix(name, ".gz") {
		gzReader, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		tarReader = gzReader
	} else if strings.HasSuffix(name, ".xz") {
		xzReader, err := xz.NewReader(reader)
		if err != nil {
			return fmt.Errorf("failed to create xz reader: %w", err)
		}
		tarReader = xzReader
	} else if strings.HasSuffix(name, ".zst") || strings.HasSuffix(name, ".zstd") {
		zstdReader, err := zstd.NewReader(reader)
		if err != nil {
			return fmt.Errorf("failed to create zstd reader: %w", err)
		}
		defer zstdReader.Close()
		tarReader = zstdReader
	} else if strings.HasSuffix(name, ".bz2") {
		tarReader = bzip2.NewReader(reader)
	}

	tr := tar.NewReader(tarReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		target := filepath.Join(targetDir, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
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

func extractSnap(archivePath, targetDir string) error {
	logger.Debug("Extracting .snap using unsquashfs")

	if _, err := exec.LookPath("unsquashfs"); err != nil {
		return fmt.Errorf("unsquashfs not found: .snap extraction requires squashfs-tools to be installed")
	}

	cmd := exec.Command("unsquashfs", "-f", "-d", targetDir, archivePath)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unsquashfs failed: %w\nOutput: %s", err, string(outputBytes))
	}

	return nil
}
