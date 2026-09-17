// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/artifactstore"
)

const (
	artifactDigestNamespace      = "sha256"
	artifactKeyVersion           = "v1"
	datadogAgentCacheDirectory   = "datadog-agent"
	authoredScriptCacheDirectory = "dd-authored-script"
)

// LocalArtifact identifies an immutable artifact directory that is ready for use.
type LocalArtifact struct {
	Directory string
}

// PackageSource materializes an authored-script package. Variant must be stable
// and identify every materialization choice not represented by the package digest.
type PackageSource interface {
	Variant() string
	Fetch(ctx context.Context, descriptor Descriptor, destination string) error
}

// PackageCache resolves descriptors to validated local artifacts.
type PackageCache struct {
	store   *artifactstore.Store
	source  PackageSource
	variant string
}

// NewPackageCache creates an authored-script package cache.
func NewPackageCache(rootDirectory string, source PackageSource) (*PackageCache, error) {
	if source == nil {
		return nil, errors.New("authored-script package source is required")
	}
	variant := source.Variant()
	if variant == "" {
		return nil, errors.New("authored-script package source variant is required")
	}
	store, err := artifactstore.New(rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not create authored-script artifact store: %w", err)
	}
	return &PackageCache{
		store:   store,
		source:  source,
		variant: variant,
	}, nil
}

// NewUserPackageCache creates a package cache rooted in the current user's
// authored-script cache directory.
func NewUserPackageCache(source PackageSource) (*PackageCache, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	return NewPackageCache(
		filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptCacheDirectory),
		source,
	)
}

// Resolve returns a validated local artifact, downloading and publishing it on
// a cache miss. The descriptor must already be authorized by the catalog.
func (c *PackageCache) Resolve(ctx context.Context, descriptor Descriptor) (LocalArtifact, error) {
	if ctx == nil {
		return LocalArtifact{}, errors.New("authored-script package resolution context is required")
	}
	if c == nil || c.store == nil || c.source == nil {
		return LocalArtifact{}, errors.New("authored-script package cache is not configured")
	}
	if err := descriptor.Validate(); err != nil {
		return LocalArtifact{}, err
	}

	artifact, err := c.store.Ensure(
		ctx,
		c.artifactKey(descriptor),
		func(ctx context.Context, destination string) error {
			return c.source.Fetch(ctx, descriptor, destination)
		},
		func(ctx context.Context, directory string) error {
			return validateCachedPackage(ctx, descriptor, directory)
		},
	)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not ensure authored-script package %q version %q: %w", descriptor.Package, descriptor.Version, err)
	}
	return LocalArtifact{Directory: artifact.Directory}, nil
}

func (c *PackageCache) artifactKey(descriptor Descriptor) artifactstore.Key {
	return artifactstore.Key{
		Namespace: artifactDigestNamespace,
		ID:        descriptor.SHA256,
		Variant:   artifactVariant(c.variant, descriptor),
	}
}

func artifactVariant(sourceVariant string, descriptor Descriptor) string {
	digest := sha256.Sum256([]byte(sourceVariant + "\x00" + descriptor.Package + "\x00" + descriptor.Version))
	return artifactKeyVersion + "-" + hex.EncodeToString(digest[:])
}

func validateCachedPackage(ctx context.Context, descriptor Descriptor, directory string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := LoadPackage(descriptor.FQN, descriptor, LocalArtifact{Directory: directory})
	if err != nil {
		return fmt.Errorf("could not validate downloaded authored-script package: %w", err)
	}
	return ctx.Err()
}
