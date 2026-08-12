// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	digestMarkerFile = ".artifact-digest"
	maxDigestSize    = 1024
)

// LocalProvider exposes pre-downloaded artifacts after their downloader publishes a
// matching digest marker. Content verification belongs to the downloader; writing the
// marker last hands a complete, immutable directory to artifact consumers.
//
// TODO: Remove this implementation when Fleet's artifact provider is available to PAR.
type LocalProvider struct {
	directories map[string]map[Platform]string
}

func NewLocalProvider(directories map[string]map[Platform]string) *LocalProvider {
	providerDirectories := make(map[string]map[Platform]string, len(directories))
	for digest, platformDirectories := range directories {
		providerDirectories[digest] = make(map[Platform]string, len(platformDirectories))
		for platform, directory := range platformDirectories {
			providerDirectories[digest][platform] = directory
		}
	}
	return &LocalProvider{directories: providerDirectories}
}

func (p *LocalProvider) Get(ctx context.Context, descriptor Descriptor) (LocalArtifact, error) {
	if ctx == nil {
		return LocalArtifact{}, errors.New("artifact provider context is required")
	}
	if p == nil {
		return LocalArtifact{}, errors.New("local artifact provider is not configured")
	}
	if err := ctx.Err(); err != nil {
		return LocalArtifact{}, err
	}

	directory, found := p.directories[descriptor.Digest][descriptor.Platform]
	if !found {
		return LocalArtifact{}, fmt.Errorf("no local directory configured for artifact %q version %q on %s/%s", descriptor.Name, descriptor.Version, descriptor.Platform.OS, descriptor.Platform.Arch)
	}
	if !filepath.IsAbs(directory) {
		return LocalArtifact{}, fmt.Errorf("local artifact directory %q is not absolute", directory)
	}

	info, err := os.Lstat(directory)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not access local artifact directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return LocalArtifact{}, fmt.Errorf("local artifact path %q is not a directory", directory)
	}
	if err := verifyDigestMarker(directory, descriptor.Digest); err != nil {
		return LocalArtifact{}, fmt.Errorf("could not verify local artifact directory %q: %w", directory, err)
	}

	return LocalArtifact{Directory: directory}, nil
}

func verifyDigestMarker(directory, expectedDigest string) error {
	markerPath := filepath.Join(directory, digestMarkerFile)
	info, err := os.Lstat(markerPath)
	if err != nil {
		return fmt.Errorf("could not access digest marker: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("digest marker is not a regular file")
	}

	file, err := os.Open(markerPath)
	if err != nil {
		return fmt.Errorf("could not open digest marker: %w", err)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maxDigestSize+1))
	if err != nil {
		return fmt.Errorf("could not read digest marker: %w", err)
	}
	if len(contents) > maxDigestSize {
		return fmt.Errorf("digest marker exceeds the %d byte limit", maxDigestSize)
	}
	if actualDigest := strings.TrimSpace(string(contents)); actualDigest != expectedDigest {
		return fmt.Errorf("digest marker %q does not match expected digest %q", actualDigest, expectedDigest)
	}
	return nil
}
