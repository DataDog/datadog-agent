// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package macpkg fetches, verifies and installs the per-version macOS .pkg an update experiment
// runs against.
//
// The artifact is always the bare, independently notarized per-version .pkg, never the .dmg. The
// .dmg is a presentation container for the human and MDM first-install path and is not involved
// in an experiment.
package macpkg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// networkRetries is how many times a download is retried on a network error. It matches the OCI
// downloader's behaviour, which is the retry budget the rest of the installer already assumes.
const networkRetries = 3

// DownloadedPackage is a .pkg materialized in scratch.
//
// Verification cannot happen before the bytes are on disk: pkgutil, codesign and spctl all
// operate on a file. What bounds the risk is that the file lives in scratch, is named by nothing
// on the host, and is deleted on any failure.
type DownloadedPackage struct {
	// Path is the .pkg's location in scratch.
	Path string
	// Version is the version the package claims to be.
	Version string
	// Digest is the hex-encoded SHA-256 of the bytes as they were written.
	Digest string

	dir string
}

// Cleanup removes the scratch directory the package was downloaded into.
func (p *DownloadedPackage) Cleanup() error {
	if p == nil || p.dir == "" {
		return nil
	}
	if err := os.RemoveAll(p.dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove scratch directory %s: %w", p.dir, err)
	}
	return nil
}

// Downloader fetches per-version .pkg artifacts.
type Downloader struct {
	client *http.Client
	env    *env.Env

	// tmpDir is the scratch root. Empty means paths.RootTmpDir; tests substitute a temporary
	// directory.
	tmpDir string
}

// NewDownloader returns a Downloader using the given HTTP client.
func NewDownloader(e *env.Env, client *http.Client) *Downloader {
	return &Downloader{client: client, env: e}
}

// Download fetches the .pkg for the given version and returns it materialized in scratch.
//
// packageURL is the URL the backend named for the version. Two forms are accepted: an explicit
// https URL ending in .pkg, which is used as given, and a registry-style reference, from which
// the .pkg URL is derived by convention. The caller is responsible for Cleanup on both the
// success and the failure path.
func (d *Downloader) Download(ctx context.Context, packageURL string, version string) (_ *DownloadedPackage, err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "macpkg.download")
	defer func() { span.Finish(err) }()
	span.SetTag("package.url", packageURL)
	span.SetTag("package.version", version)

	resolved, err := d.resolveURL(packageURL, version)
	if err != nil {
		return nil, err
	}

	root := d.tmpDir
	if root == "" {
		root = paths.RootTmpDir
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("could not create scratch root %s: %w", root, err)
	}
	dir, err := os.MkdirTemp(root, "macpkg-")
	if err != nil {
		return nil, fmt.Errorf("could not create scratch directory: %w", err)
	}
	pkg := &DownloadedPackage{
		Path:    path.Join(dir, "datadog-agent.pkg"),
		Version: version,
		dir:     dir,
	}
	// Any failure from here on deletes the scratch directory, so a partial or unverifiable
	// download leaves nothing behind for a later run to trip over.
	defer func() {
		if err != nil {
			if cleanupErr := pkg.Cleanup(); cleanupErr != nil {
				log.Warnf("could not clean up after a failed download: %v", cleanupErr)
			}
		}
	}()

	digest, err := d.fetch(ctx, resolved, pkg.Path)
	if err != nil {
		return nil, err
	}
	pkg.Digest = digest
	log.Infof("Downloaded %s (%s) to %s", resolved, digest, pkg.Path)
	return pkg, nil
}

// resolveURL turns what the backend named into the https URL of the per-version .pkg.
//
// An https URL that already names a .pkg is used unchanged, which is what a mirror or a manually
// driven experiment provides. Anything else is treated as a registry-style reference and mapped
// onto the mirror or the site by convention, because the OCI index is selected by GOOS and
// carries no darwin image.
func (d *Downloader) resolveURL(packageURL string, version string) (string, error) {
	if version == "" {
		return "", errors.New("a version is required to download a macOS package")
	}
	if parsed, err := url.Parse(packageURL); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
		if strings.HasSuffix(parsed.Path, ".pkg") {
			return packageURL, nil
		}
		return joinURL(packageURL, pkgFileName(version)), nil
	}
	base := ""
	if d.env != nil {
		base = d.env.Mirror
	}
	if base == "" {
		return "", fmt.Errorf("cannot resolve a macOS package URL for version %s from %q: set installer.mirror, or have the backend name an https .pkg URL", version, packageURL)
	}
	return joinURL(base, pkgFileName(version)), nil
}

func pkgFileName(version string) string {
	return fmt.Sprintf("datadog-agent-%s.pkg", version)
}

func joinURL(base string, file string) string {
	return strings.TrimSuffix(base, "/") + "/" + file
}

// fetch writes the URL's body to targetPath and returns its hex-encoded SHA-256.
//
// The digest is computed from the bytes as they are written rather than by re-reading the file,
// so what is reported is what landed.
func (d *Downloader) fetch(ctx context.Context, resolved string, targetPath string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < networkRetries; attempt++ {
		if attempt > 0 {
			log.Warnf("retrying the download of %s after a network error: %v", resolved, lastErr)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Second):
			}
		}
		digest, err := d.fetchOnce(ctx, resolved, targetPath)
		if err == nil {
			return digest, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not download %s after %d attempts: %w", resolved, networkRetries, lastErr)
}

func (d *Downloader) fetchOnce(ctx context.Context, resolved string, targetPath string) (_ string, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
	if err != nil {
		return "", fmt.Errorf("could not build the request for %s: %w", resolved, err)
	}
	client := d.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("could not fetch %s: %w", resolved, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", &httpError{URL: resolved, StatusCode: response.StatusCode}
	}

	// O_EXCL: a retry must not append to what a previous attempt left behind.
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("could not clear %s before writing: %w", targetPath, err)
	}
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("could not create %s: %w", targetPath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("could not close %s: %w", targetPath, closeErr)
		}
	}()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), response.Body); err != nil {
		return "", fmt.Errorf("could not write %s: %w", targetPath, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// httpError is a non-200 response. It is separated out so the retry decision can distinguish a
// 503 from a 404 without string matching.
type httpError struct {
	URL        string
	StatusCode int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("%s returned %d", e.URL, e.StatusCode)
}

func isRetryable(err error) bool {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == http.StatusTooManyRequests
	}
	// A context cancellation is a decision, not a transient failure.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Anything else at this layer is a transport failure: the only non-transport errors above
	// are filesystem ones, and those are reported against a scratch directory this process just
	// created, so retrying them is harmless.
	return true
}
