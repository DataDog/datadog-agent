// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact"
)

// Downloader materializes authored-script artifacts through the shared
// content-addressed artifact manager.
type Downloader struct {
	manager  *artifact.Manager
	platform artifact.Platform
}

// NewDownloader creates a Downloader using a dedicated artifact cache root.
func NewDownloader(root string, platform artifact.Platform, materializer artifact.Materializer) (*Downloader, error) {
	if platform.OS == "" || platform.Architecture == "" {
		return nil, errors.New("authored-script download platform is required")
	}
	manager, err := artifact.NewManager(root, materializer)
	if err != nil {
		return nil, fmt.Errorf("could not create authored-script artifact manager: %w", err)
	}
	return &Downloader{manager: manager, platform: platform}, nil
}

// NewUserCacheDownloader creates a Downloader below the current user's cache.
func NewUserCacheDownloader(platform artifact.Platform, materializer artifact.Materializer) (*Downloader, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	return NewDownloader(filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptDirectory), platform, materializer)
}

// Download returns a validated local artifact. Catalog authorization must be
// checked separately immediately before execution, including on cache hits.
func (d *Downloader) Download(ctx context.Context, descriptor Descriptor) (LocalArtifact, error) {
	if ctx == nil {
		return LocalArtifact{}, errors.New("authored-script download context is required")
	}
	if d == nil || d.manager == nil {
		return LocalArtifact{}, errors.New("authored-script downloader is not configured")
	}
	sharedDescriptor, err := descriptor.artifactDescriptor(d.platform)
	if err != nil {
		return LocalArtifact{}, err
	}
	handle, err := d.manager.Ensure(ctx, sharedDescriptor)
	if err != nil {
		return LocalArtifact{}, err
	}
	return LocalArtifact{Directory: handle.Directory}, nil
}
