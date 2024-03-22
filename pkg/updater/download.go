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
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ociLayoutArchiveName          = "oci-layout.tar"
	ociLayoutName                 = "oci-layout"
	ociLayoutArchiveMaxSize int64 = 1 << 30 // 1GiB
	ociLayoutMaxSize        int64 = 1 << 30 // 1GiB
)

// downloader is the downloader used by the updater to download packages.
type downloader struct {
	client        *http.Client
	remoteBaseURL string
}

// newDownloader returns a new Downloader.
func newDownloader(client *http.Client, remoteBaseURL string) *downloader {
	return &downloader{
		client:        client,
		remoteBaseURL: remoteBaseURL,
	}
}

// Download downloads the Datadog Package referenced in the given Package struct.
func (d *downloader) Download(ctx context.Context, tmpDir string, pkg Package) (oci.Image, error) {
	log.Debugf("Downloading package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	url, err := url.Parse(pkg.URL)
	if err != nil {
		return nil, fmt.Errorf("could not parse package URL: %w", err)
	}
	var image oci.Image
	switch url.Scheme {
	case "http", "https":
		image, err = d.downloadHTTP(ctx, pkg.URL, pkg.SHA256, pkg.Size, tmpDir)
	case "oci":
		image, err = d.downloadRegistry(ctx, pkg)
	default:
		return nil, fmt.Errorf("unsupported package URL scheme: %s", url.Scheme)
	}
	if err != nil {
		return nil, fmt.Errorf("could not download package from %s: %w", pkg.URL, err)
	}
	log.Debugf("Successfully downloaded package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	return image, nil
}

func (d *downloader) getRegistryURL(pkg Package) string {
	downloadURL := strings.TrimPrefix(pkg.URL, "oci://")
	if d.remoteBaseURL != "" {
		remoteBaseURL := d.remoteBaseURL
		if !strings.HasSuffix(d.remoteBaseURL, "/") {
			remoteBaseURL += "/"
		}
		split := strings.Split(pkg.URL, "/")
		downloadURL = remoteBaseURL + split[len(split)-1]
	}
	return downloadURL
}

func (d *downloader) downloadRegistry(ctx context.Context, pkg Package) (oci.Image, error) {
	url := d.getRegistryURL(pkg)

	// the image URL is parsed as a digest to ensure we use the <repository>/<image>@<digest> format
	digest, err := name.NewDigest(url, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("could not parse digest: %w", err)
	}

	platform := oci.Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
	index, err := remote.Index(digest, remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("could not download image: %w", err)
	}
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("could not get index manifest: %w", err)
	}
	for _, manifest := range indexManifest.Manifests {
		if manifest.Platform != nil && !manifest.Platform.Satisfies(platform) {
			continue
		}
		image, err := index.Image(manifest.Digest)
		if err != nil {
			return nil, fmt.Errorf("could not get image: %w", err)
		}
		return image, nil
	}
	return nil, fmt.Errorf("no matching image found in the index")
}

func (d *downloader) downloadHTTP(ctx context.Context, url string, sha256hash string, size int64, tmpDir string) (oci.Image, error) {
	// Request the oci-layout.tar archive from the given URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create download request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not download package: %w", err)
	}
	defer resp.Body.Close()
	if resp.ContentLength != -1 && resp.ContentLength != size {
		return nil, fmt.Errorf("invalid size: expected %d, got %d", size, resp.ContentLength)
	}

	// Write oci-layout.tar on disk and check its hash while doing so
	hashWriter := sha256.New()
	reader := io.TeeReader(
		io.LimitReader(resp.Body, ociLayoutArchiveMaxSize),
		hashWriter,
	)
	ociLayoutArchiveFile, err := os.Create(filepath.Join(tmpDir, ociLayoutArchiveName))
	if err != nil {
		return nil, fmt.Errorf("could not create oci layout archive: %w", err)
	}
	defer ociLayoutArchiveFile.Close()
	_, err = io.Copy(ociLayoutArchiveFile, reader)
	if err != nil {
		return nil, fmt.Errorf("could not write oci layout archive: %w", err)
	}
	computedHash := hashWriter.Sum(nil)
	expectedHash, err := hex.DecodeString(sha256hash)
	if err != nil {
		return nil, fmt.Errorf("could not decode hash: %w", err)
	}
	if !bytes.Equal(expectedHash, computedHash) {
		return nil, fmt.Errorf("invalid hash: expected %s, got %x", sha256hash, computedHash)
	}

	// Extract oci-layout.tar to the oci-layout directory
	ociLayoutPath := filepath.Join(tmpDir, ociLayoutName)
	err = os.Mkdir(ociLayoutPath, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create oci layout directory: %w", err)
	}
	_, err = ociLayoutArchiveFile.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("could not seek to the beginning of the oci layout archive: %w", err)
	}
	err = extractTarArchive(ociLayoutArchiveFile, ociLayoutPath, ociLayoutMaxSize)
	if err != nil {
		return nil, fmt.Errorf("could not extract oci layout archive: %w", err)
	}

	// Load the oci-layout directory as an oci image
	platform := oci.Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
	index, err := layout.ImageIndexFromPath(ociLayoutPath)
	if err != nil {
		return nil, fmt.Errorf("could not load oci layout image index: %w", err)
	}
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("could not get index manifest: %w", err)
	}
	for _, manifest := range indexManifest.Manifests {
		if manifest.Platform != nil && !manifest.Platform.Satisfies(platform) {
			continue
		}
		image, err := index.Image(manifest.Digest)
		if err != nil {
			return nil, fmt.Errorf("could not get image: %w", err)
		}
		return image, nil
	}
	return nil, fmt.Errorf("no matching image found in the index")
}

// extractTarArchive extracts a tar archive to the given destination path
//
// Note on security: This function does not currently attempt to fully mitigate zip-slip attacks.
// This is purposeful as the archive is extracted only after its SHA256 hash has been validated
// against its reference in the package catalog. This catalog is itself sent over Remote Config
// which guarantees its integrity.
func extractTarArchive(reader io.Reader, destinationPath string, maxSize int64) error {
	log.Debugf("Extracting archive to %s", destinationPath)
	tr := tar.NewReader(io.LimitReader(reader, maxSize))
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
			err = os.MkdirAll(target, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		case tar.TypeReg:
			err = extractTarFile(target, tr, os.FileMode(header.Mode))
			if err != nil {
				return err // already wrapped
			}
		case tar.TypeSymlink:
			err = os.Symlink(header.Linkname, target)
			if err != nil {
				return fmt.Errorf("could not create symlink: %w", err)
			}
		case tar.TypeLink:
			// we currently don't support hard links in the updater
		default:
			log.Warnf("Unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}

	log.Debugf("Successfully extracted archive to %s", destinationPath)
	return nil
}

// extractTarFile extracts a file from a tar archive.
// It is separated from extractTarGz to ensure `defer f.Close()` is called right after the file is written.
func extractTarFile(targetPath string, reader io.Reader, mode fs.FileMode) error {
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	f, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
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
