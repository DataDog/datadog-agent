// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentArchiveFileName       = "agent.tar.gz"
	maxArchiveSize             = 5 << 30  // 5GiB
	maxArchiveDecompressedSize = 10 << 30 // 10GiB
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
// It currently assumes the package is a tar.gz archive.
func (d *downloader) Download(ctx context.Context, pkg Package, destinationPath string) error {
	log.Debugf("Downloading package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)

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

	// Download archive
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
	if err != nil {
		return fmt.Errorf("could not create download request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	defer resp.Body.Close()
	if resp.ContentLength != pkg.Size {
		return fmt.Errorf("invalid size for %s: expected %d, got %d", pkg.URL, pkg.Size, resp.ContentLength)
	}

	// Write archive on disk & check its hash while doing so
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
	sha256 := hashWriter.Sum(nil)
	expectedHash, err := hex.DecodeString(pkg.SHA256)
	if err != nil {
		return fmt.Errorf("could not decode hash: %w", err)
	}
	if !bytes.Equal(expectedHash, sha256) {
		return fmt.Errorf("invalid hash for %s: expected %s, got %x", pkg.URL, pkg.SHA256, sha256)
	}

	// Extract the OCI archive
	extractedArchivePath := filepath.Join(tmpDir, "archive")
	if err := os.Mkdir(extractedArchivePath, 0755); err != nil {
		return fmt.Errorf("could not create archive extraction directory: %w", err)
	}
	extractedOCIPath := filepath.Join(tmpDir, "oci")
	if err := os.Mkdir(extractedOCIPath, 0755); err != nil {
		return fmt.Errorf("could not create OCI extraction directory: %w", err)
	}
	err = extractTarGz(archivePath, extractedArchivePath)
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	err = extractOCI(extractedArchivePath, extractedOCIPath)
	if err != nil {
		return fmt.Errorf("could not extract OCI archive: %w", err)
	}

	// Only keep the package directory /opt/datadog-packages/<package-name>/<package-version>
	// Other files and directory in the archive are ignored
	packageDir := filepath.Join(extractedOCIPath, defaultRepositoryPath, pkg.Name, pkg.Version)
	err = os.Rename(packageDir, destinationPath)
	if err != nil {
		return fmt.Errorf("could not move package directory: %w", err)
	}

	log.Debugf("Successfully downloaded package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	return nil
}

// extractTarGz extracts a tar.gz archive to the given destination path
//
// Note on security: This function does not currently attempt to fully mitigate zip-slip attacks.
// This is purposeful as the archive is extracted only after its SHA256 hash has been validated
// against its reference in the package catalog. This catalog is itself sent over Remote Config
// which guarantees its integrity.
func extractTarGz(archivePath string, destinationPath string) error {
	log.Debugf("Extracting archive %s to %s", archivePath, destinationPath)

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("could not create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(io.LimitReader(gzr, maxArchiveDecompressedSize))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}
		if header.Name == "./" {
			continue
		}

		target := filepath.Join(destinationPath, header.Name)

		// Check for directory traversal. Note that this is more of a sanity check than a security measure.
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
			err = extractTarFile(target, tr)
			if err != nil {
				return err // already wrapped
			}
		case tar.TypeSymlink:
			err = os.Symlink(header.Linkname, target)
			if err != nil {
				return fmt.Errorf("could not create symlink: %w", err)
			}
		default:
			log.Warnf("Unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}

	log.Debugf("Successfully extracted archive %s to %s", archivePath, destinationPath)
	return nil
}

// extractTarFile extracts a file from a tar archive.
// It is separated from extractTarGz to ensure `defer f.Close()` is called right after the file is written.
func extractTarFile(targetPath string, reader io.Reader) error {
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("could not write file: %w", err)
	}
	return nil
}
