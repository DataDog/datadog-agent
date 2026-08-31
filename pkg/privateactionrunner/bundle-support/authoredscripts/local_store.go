// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/go-digest"
)

const (
	datadogAgentCacheDirectory = "datadog-agent"
	authoredScriptDirectory    = "dd-authored-script"
)

// LocalArtifact identifies an immutable artifact directory that is ready for use.
type LocalArtifact struct {
	Directory string
}

var ErrLocalArtifactNotFound = errors.New("authored-script artifact is not available locally")

type LocalStore struct {
	rootDirectory string
}

func NewLocalStore(rootDirectory string) (*LocalStore, error) {
	if rootDirectory == "" {
		return nil, errors.New("local artifact store directory is required")
	}
	if !filepath.IsAbs(rootDirectory) {
		return nil, fmt.Errorf("local artifact store directory %q is not absolute", rootDirectory)
	}
	return &LocalStore{rootDirectory: filepath.Clean(rootDirectory)}, nil
}

func NewUserCacheStore() (*LocalStore, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	return NewLocalStore(filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptDirectory))
}

func (s *LocalStore) Open(descriptor Descriptor) (LocalArtifact, error) {
	if s == nil || s.rootDirectory == "" {
		return LocalArtifact{}, errors.New("local artifact store is not configured")
	}
	if err := validateDescriptor(descriptor); err != nil {
		return LocalArtifact{}, err
	}

	artifactDigest := digest.NewDigestFromEncoded(digest.SHA256, descriptor.SHA256)
	if err := artifactDigest.Validate(); err != nil {
		return LocalArtifact{}, fmt.Errorf("invalid authored-script SHA-256 digest %q: %w", descriptor.SHA256, err)
	}

	artifactDirectory, err := securejoin.SecureJoin(s.rootDirectory, artifactDigest.Encoded())
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not resolve local authored-script artifact directory: %w", err)
	}
	scriptPath, err := securejoin.SecureJoin(artifactDirectory, scriptDirectory)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not resolve local authored-script directory: %w", err)
	}
	info, err := os.Lstat(scriptPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LocalArtifact{}, fmt.Errorf("%w: package %q version %q at %q", ErrLocalArtifactNotFound, descriptor.Package, descriptor.Version, scriptPath)
		}
		return LocalArtifact{}, fmt.Errorf("could not open authored-script artifact %q: %w", scriptPath, err)
	}
	if !info.IsDir() {
		return LocalArtifact{}, fmt.Errorf("authored-script artifact path %q is not a directory", scriptPath)
	}

	return LocalArtifact{Directory: artifactDirectory}, nil
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.Package == "" {
		return errors.New("authored-script package is required")
	}
	if descriptor.Version == "" {
		return errors.New("authored-script version is required")
	}
	if descriptor.URL == "" {
		return errors.New("authored-script URL is required")
	}
	if descriptor.SHA256 == "" {
		return errors.New("authored-script SHA-256 digest is required")
	}
	return nil
}
