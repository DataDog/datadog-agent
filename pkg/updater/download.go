// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/go-secure-sdk/compression/archive/tar"
	"github.com/xi2/xz"
)

const (
	agentArchiveFileName       = "agent.tar.gz"
	maxArchiveSize             = 5 << 30  // 10GB
	maxArchiveDecompressedSize = 10 << 30 // 1GB
	maxArchiveFileSize         = 1 << 30  // 1GB
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
// It currently assumes the package is a tar.gz archive.
func (d *downloader) Download(ctx context.Context, pkg Package, destinationPath string) error {
	log.Debugf("Downloading package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)

	// Check platform and architecture compatibility
	if pkg.Platform != runtime.GOOS {
		return fmt.Errorf("unsupported platform %s for package %s", pkg.Platform, pkg.Name)
	}
	if pkg.Arch != runtime.GOARCH {
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

	// Execute any additional operation on the extracted archive, depending on the package name
	switch pkg.Name {
	case "datadog-agent":
		// Only extract /opt/datadog-agent from the OCI archive
		err = os.Rename(filepath.Join(extractedOCIPath, defaultRepositoryPath, pkg.Name, pkg.Version), destinationPath)
		if err != nil {
			return fmt.Errorf("could not move OCI archive: %w", err)
		}
	default:
		// By default, move the entire extracted archive to the destination path
		err = os.Rename(extractedOCIPath, destinationPath)
		if err != nil {
			return fmt.Errorf("could not move OCI archive: %w", err)
		}
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
	xzr, err := xz.NewReader(f, 0)
	if err != nil {
		return fmt.Errorf("could not create gzip reader: %w", err)
	}

	// Extract tar archive
	err = tar.Extract(
		xzr,
		destinationPath,
		tar.WithMaxArchiveSize(maxArchiveDecompressedSize),
		tar.WithMaxSymlinkRecursion(maxArchiveLinkDepth),
		tar.WithMaxFileSize(maxArchiveFileSize),
		tar.WithMaxEntryCount(maxArchiveFileCount),
	)
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}

	log.Debugf("Successfully extracted archive %s to %s", archivePath, destinationPath)
	return nil
}
