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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentArchiveFileName = "agent.tar.gz"
	maxArchiveSize       = 1 << 30 // 1GB
	maxDecompressedSize  = 1 << 30 // 1GB
	maxFileSize          = 1 << 30 // 1GB
	maxFileCount         = 1000
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
	if err != nil {
		return fmt.Errorf("could not create download request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	defer resp.Body.Close()
	hashWriter := sha256.New()
	reader := io.TeeReader(io.LimitReader(req.Body, maxArchiveSize), hashWriter)
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
		return fmt.Errorf("invalid hash for %s: expected %x, got %x", pkg.URL, pkg.SHA256, sha256)
	}
	err = extractTarGz(archivePath, destinationPath)
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	log.Debugf("Successfully downloaded package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	return nil
}

// extractTarGz extracts a tar.gz archive to the given destination path.
//
// Note on security: This function does not currently attempt to mitigate zip-slip attacks.
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

	// Limit the number of files and the size of the decompressed archive
	archiveFileCount := 0
	archiveReader := io.LimitReader(gzr, maxDecompressedSize)
	tr := tar.NewReader(archiveReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}
		cleanName := filepath.Clean(header.Name)
		target := filepath.Join(destinationPath, cleanName)

		// Check for zip-slip attacks
		outPath, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("could not get absolute path for %s: %w", cleanName, err)
		}
		// Ensure the file is within the destination directory
		if !strings.HasPrefix(outPath, destinationPath) {
			return fmt.Errorf("tar entry %s is trying to escape the destination directory", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
			if err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		case tar.TypeReg:
			archiveFileCount++
			if archiveFileCount > maxFileCount {
				return errors.New("archive contains too many files")
			}
			err = extractTarGzFile(target, tr)
			if err != nil {
				return err // already wrapped
			}
		default:
			log.Warnf("Unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}
	log.Debugf("Successfully extracted archive %s to %s", archivePath, destinationPath)
	return nil
}

// extractTarGzFile extracts a file from a tar.gz archive.
// It is separated from extractTarGz to ensure `defer f.Close()` is called right after the file is written.
func extractTarGzFile(targetPath string, reader io.Reader) error {
	err := os.MkdirAll(filepath.Dir(targetPath), 0o755)
	if err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()
	limitedReader := io.LimitReader(reader, maxFileSize)
	n, err := io.Copy(f, limitedReader)
	if err != nil {
		if errors.Is(err, io.EOF) && n == maxFileSize {
			return fmt.Errorf("content truncated: file %q is too large: %w", targetPath, err)
		}
		return fmt.Errorf("could not write file: %w", err)
	}
	return nil
}
