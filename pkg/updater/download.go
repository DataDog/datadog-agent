// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/ulikunitz/xz"
)

const (
	agentArchiveFileName       = "agent.tar.xz"
	maxArchiveSize             = 5 << 30  // 5GiB
	maxArchiveDecompressedSize = 10 << 30 // 10GiB
	maxArchiveFileSize         = 1 << 30  // 1GiB
	maxArchiveFileCount        = 50_000
	maxArchiveLinkDepth        = 5
)

// downloader is the downloader used by the updater to download packages.
type downloader struct {
	client *http.Client
}

// newDownloader returns a new Downloader.
func newDownloader(client *http.Client) *downloader {
	return &downloader{
		client: client,
	}
}

// Download downloads the package at the given URL in temporary directory,
// verifies its SHA256 hash and extracts it to the given destination path.
// It currently assumes the package is a tar.xz archive.
func (d *downloader) Download(ctx context.Context, pkg Package, destinationPath string) error {
	log.Debugf("Downloading package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)

	// Check platform and architecture compatibility
	if pkg.Platform != "" && pkg.Platform != runtime.GOOS {
		return fmt.Errorf("unsupported platform %s for package %s", pkg.Platform, pkg.Name)
	}
	if pkg.Arch != "" && pkg.Arch != runtime.GOARCH {
		return fmt.Errorf("unsupported architecture %s for package %s", pkg.Arch, pkg.Name)
	}

	// Create temporary directory to download the archive
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			log.Errorf("could not cleanup temporary directory: %v", err)
		}
	}()

	// Get archive
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
	if err != nil {
		return fmt.Errorf("could not create download request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	defer resp.Body.Close()

	// Verify content length
	if resp.ContentLength != pkg.Size {
		return fmt.Errorf("invalid size for %s: expected %d, got %d", pkg.URL, pkg.Size, resp.ContentLength)
	}

	// Copy archive & build hash
	hashWriter := sha256.New()
	reader := io.TeeReader(
		io.LimitReader(resp.Body, maxArchiveSize),
		hashWriter,
	)
	archivePath := filepath.Join(tmpDir, agentArchiveFileName)
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("could not create archive file: %w", err)
	}
	defer archiveFile.Close()
	_, err = io.Copy(archiveFile, reader)
	if err != nil {
		return fmt.Errorf("could not write archive file: %w", err)
	}

	// Verify hash
	sha256 := hashWriter.Sum(nil)
	expectedHash, err := hex.DecodeString(pkg.SHA256)
	if err != nil {
		return fmt.Errorf("could not decode hash: %w", err)
	}
	if !bytes.Equal(expectedHash, sha256) {
		return fmt.Errorf("invalid hash for %s: expected %s, got %x", pkg.URL, pkg.SHA256, sha256)
	}

	// Extract OCI archive to a temporary directory
	extractedArchivePath := filepath.Join(tmpDir, "oci")
	if err := os.Mkdir(extractedArchivePath, 0755); err != nil {
		return fmt.Errorf("could not create archive extraction directory: %w", err)
	}

	err = extractTarXz(archivePath, extractedArchivePath)
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}

	// Extract package from OCI archive
	extractedOCIPath := filepath.Join(tmpDir, "extracted")
	if err := os.Mkdir(extractedOCIPath, 0755); err != nil {
		return fmt.Errorf("could not create OCI extraction directory: %w", err)
	}

	err = extractOCI(extractedArchivePath, extractedOCIPath)
	if err != nil {
		return fmt.Errorf("could not extract OCI archive: %w", err)
	}

	// As we are extracting into a temporary path and we can't Rename to an existing path,
	// we need to remove the existing destination path. It also lets us make sure that the
	// destination path is not in a half-extracted state and only contains the new version.
	err = os.RemoveAll(destinationPath)
	if err != nil {
		return fmt.Errorf("could not remove existing destination path: %w", err)
	}

	// We only need to extract /opt/datadog-packages/<package-name>/<package-version> from the OCI archive today.
	// We can add more logic here if we need to extract more files or directories in the future.
	err = os.Rename(filepath.Join(extractedOCIPath, defaultRepositoryPath, pkg.Name, pkg.Version), destinationPath)
	if err != nil {
		return fmt.Errorf("could not move OCI archive: %w", err)
	}

	log.Debugf("Successfully downloaded package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	return nil
}

// extractTarXz extracts a tar.xz archive to the given destination path
func extractTarXz(archivePath string, destinationPath string) error {
	log.Debugf("Extracting archive %s to %s", archivePath, destinationPath)

	// Read XZ archive
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	defer f.Close()
	xzr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("could not create gzip reader: %w", err)
	}

	// Extract tar archive
	tr := tar.NewReader(io.LimitReader(xzr, maxArchiveDecompressedSize))
	tarLinks := make([]*tar.Header, 0)
	fileCount := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}

		// Skip the root directory
		if header.Name == "./" {
			continue
		}

		target := filepath.Join(destinationPath, header.Name)

		// Check for zip-slip attacks
		if !strings.HasPrefix(target, filepath.Clean(destinationPath)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %s is trying to escape the destination directory", header.Name)
		}

		// Extract element depending on its type
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0755)
			if err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		case tar.TypeReg:
			fileCount++
			if fileCount > maxArchiveFileCount {
				return errors.New("archive contains too many files")
			}
			err = extractTarFile(target, tr)
			if err != nil {
				return err // already wrapped
			}
		case tar.TypeLink, tar.TypeSymlink:
			tarLinks = append(tarLinks, header)
		default:
			log.Warnf("Unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}

	// Process tar links afterwards as they may depend on other files being written
	err = processTarLinks(0, destinationPath, tarLinks)
	if err != nil {
		return err // already wrapped
	}

	log.Debugf("Successfully extracted archive %s to %s", archivePath, destinationPath)
	return nil
}

// extractTarFile extracts a file from a tar archive.
// It is separated from extractTarGz to ensure `defer f.Close()` is called right after the file is written.
func extractTarFile(targetPath string, reader io.Reader) error {
	err := os.MkdirAll(filepath.Dir(targetPath), 0o755)
	if err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	f, err := os.Create(filepath.FromSlash(targetPath))
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, io.LimitReader(reader, maxArchiveFileSize))
	if err != nil {
		if errors.Is(err, io.EOF) && n == maxArchiveFileSize {
			defer func() {
				if err := os.Remove(targetPath); err != nil {
					log.Errorf("Could not remove truncated file %q: %v", targetPath, err)
				} else {
					log.Debug("Removing truncated file %q", targetPath)
				}
			}()
			return fmt.Errorf("content truncated: file %q is too large: %w", targetPath, err)
		}
		return fmt.Errorf("could not write file: %w", err)
	}
	return nil
}

// processTarLinks processes the symlinks in a tar archive and makes sure the destinations exist.
// It is called recursively to handle symlinks that depend on other symlinks as we
// iteratively create them.
func processTarLinks(level int, destinationPath string, symlinks []*tar.Header) error {
	if len(symlinks) == 0 {
		// Fast path
		return nil
	}

	// Check recursion level
	if level >= maxArchiveLinkDepth {
		return fmt.Errorf("maximum symlink recursion level reached (%d)", level)
	}

	// Prepare next pass
	next := make([]*tar.Header, 0)

	for _, hdr := range symlinks {
		if hdr == nil {
			continue
		}

		targetName := filepath.Join(destinationPath, hdr.Name)

		// Generate the target link name depending on whether it is absolute
		// or relative to the current file. Hardlinks are always absolute
		var targetLinkName string
		if filepath.IsAbs(hdr.Linkname) || hdr.Typeflag == tar.TypeLink {
			targetLinkName = filepath.Join(destinationPath, hdr.Linkname)
		} else {
			targetLinkName = filepath.Join(filepath.Dir(targetName), hdr.Linkname)
		}

		// Check for zip-slip attacks
		if !strings.HasPrefix(targetLinkName, filepath.Clean(destinationPath)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %s is trying to escape the destination directory", hdr.Name)
		}

		// Check if the target link already exists, if not, add to next pass
		if _, err := os.Stat(filepath.FromSlash(targetLinkName)); err != nil {
			next = append(next, hdr)
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeLink: // Hard link
			if err := os.Link(filepath.FromSlash(targetLinkName), filepath.FromSlash(targetName)); err != nil {
				return fmt.Errorf("unable to create hardlink: %w", err)
			}
		case tar.TypeSymlink:
			if err := os.Symlink(filepath.FromSlash(targetLinkName), filepath.FromSlash(targetName)); err != nil {
				return fmt.Errorf("unable to create symlink: %w", err)
			}
		}
	}

	// Process next pass
	if len(next) > 0 {
		return processTarLinks(level+1, destinationPath, next)
	}

	return nil
}
