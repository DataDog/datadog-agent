// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packaging

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mholt/archiver/v3"
)

const (
	agentArchiveFileName = "agent.tar.gz"
)

// Downloader is the downloader used by the updater to download packages.
type Downloader struct {
	client *http.Client
}

// NewDownloader returns a new Downloader.
func NewDownloader(client *http.Client) *Downloader {
	return &Downloader{
		client: client,
	}
}

// Download downloads the package at the given URL in temporary directory,
// verifies its SHA256 hash and extracts it to the given destination path.
// It currently assumes the package is a tar.gz archive.
func (d *Downloader) Download(ctx context.Context, url string, expectedSHA256 []byte, destinationPath string) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("could not create download request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	defer resp.Body.Close()
	hashWriter := sha256.New()
	reader := io.TeeReader(resp.Body, hashWriter)
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
	if !bytes.Equal(expectedSHA256, sha256) {
		return fmt.Errorf("invalid hash for %s: expected %x, got %x", url, expectedSHA256, sha256)
	}
	err = archiver.Extract(archivePath, "", destinationPath)
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	return nil
}
