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

	// Extract OCI archive to temporary directory
	ociExtractionPath := filepath.Join(tmpDir, "oci")
	if err := os.Mkdir(ociExtractionPath, 0755); err != nil {
		return fmt.Errorf("could not create OCI extraction directory: %w", err)
	}

	err = extractTarXz(archivePath, ociExtractionPath)
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}

	// Extract package from OCI archive
	err = extractOCI(ociExtractionPath, destinationPath)
	if err != nil {
		return fmt.Errorf("could not extract OCI archive: %w", err)
	}

	log.Debugf("Successfully downloaded package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	return nil
}

// extractTarXz extracts a tar.xz archive to the given destination path.
//
// Note on security: This function does not currently attempt to mitigate zip-slip attacks.
// This is purposeful as the archive is extracted only after its SHA256 hash has been validated
// against its reference in the package catalog. This catalog is itself sent over Remote Config
// which guarantees its integrity.
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
