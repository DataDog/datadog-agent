// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifacts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"
	digest "github.com/opencontainers/go-digest"
)

const (
	datadogAgentCacheDirectory = "datadog-agent"
	artifactsCacheDirectory    = "artifacts"
	objectsDirectory           = "objects"
)

// LocalArtifact identifies an immutable artifact directory that is ready for use.
type LocalArtifact struct {
	Directory string
}

// LocalStore opens fully populated artifacts from
// <root>/objects/<digest>/<os>/<arch>. It does not download artifacts or manage
// cache publication.
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
	return NewLocalStore(filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, artifactsCacheDirectory))
}

func (s *LocalStore) Open(descriptor Descriptor) (LocalArtifact, error) {
	if s == nil {
		return LocalArtifact{}, errors.New("local artifact store is not configured")
	}
	if err := validateDescriptor(descriptor); err != nil {
		return LocalArtifact{}, err
	}

	parsedDigest, err := digest.Parse(descriptor.Digest)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("invalid artifact digest %q: %w", descriptor.Digest, err)
	}
	digestDirectory := parsedDigest.Algorithm().String() + "-" + parsedDigest.Encoded()
	directory, err := securejoin.SecureJoin(
		s.rootDirectory,
		filepath.Join(
			objectsDirectory,
			digestDirectory,
			descriptor.Platform.OS,
			descriptor.Platform.Arch,
		),
	)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not resolve local artifact directory: %w", err)
	}

	info, err := os.Lstat(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LocalArtifact{}, fmt.Errorf("%w: artifact %q version %q for %s/%s is not available at %q", ErrNotFound, descriptor.Name, descriptor.Version, descriptor.Platform.OS, descriptor.Platform.Arch, directory)
		}
		return LocalArtifact{}, fmt.Errorf("could not access local artifact directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return LocalArtifact{}, fmt.Errorf("local artifact path %q is not a directory", directory)
	}
	return LocalArtifact{Directory: directory}, nil
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.Name == "" {
		return errors.New("artifact name is required")
	}
	if descriptor.Version == "" {
		return errors.New("artifact version is required")
	}
	if descriptor.URL == "" {
		return errors.New("artifact URL is required")
	}
	if descriptor.Digest == "" {
		return errors.New("artifact digest is required")
	}
	if !isPathComponent(descriptor.Platform.OS) {
		return fmt.Errorf("invalid artifact operating system %q", descriptor.Platform.OS)
	}
	if !isPathComponent(descriptor.Platform.Arch) {
		return fmt.Errorf("invalid artifact architecture %q", descriptor.Platform.Arch)
	}
	return nil
}

func isPathComponent(value string) bool {
	return value != "" && filepath.IsLocal(value) && filepath.Base(value) == value
}
