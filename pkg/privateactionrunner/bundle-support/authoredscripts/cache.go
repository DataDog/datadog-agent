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

	godigest "github.com/opencontainers/go-digest"

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

// PackageSource fetches one authored-script package into destination.
// Implementations own source-specific work such as downloading an OCI image
// and selecting, extracting, and validating its package layer. Fetch must
// return nil only when destination contains the complete source output, and it
// must not write outside destination.
//
// Variant must identify every source or materialization choice not represented
// by Descriptor.SHA256, including platform, flavor, and output-layout version.
// Both methods must be stable across processes, and Fetch must be safe to call
// concurrently for different packages.
type PackageSource interface {
	Variant() string
	Fetch(ctx context.Context, descriptor Descriptor, destination string) error
}

// PackageCache resolves authored-script descriptors to validated local
// artifacts. It is safe for concurrent use. It does not perform catalog lookup
// or decide whether a package is authorized.
type PackageCache struct {
	store   *artifactstore.Store
	source  PackageSource
	variant string
}

// NewPackageCache creates an authored-script package cache. rootDirectory must
// be a dedicated, trusted local-filesystem directory; see artifactstore.New for
// the complete ownership and filesystem requirements.
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

// Resolve returns a validated local artifact for descriptor. On a cache miss,
// it fetches the package into staging and atomically publishes the validated
// result. Descriptor is assumed to have been authorized by an upstream catalog
// boundary.
func (c *PackageCache) Resolve(ctx context.Context, descriptor Descriptor) (LocalArtifact, error) {
	if ctx == nil {
		return LocalArtifact{}, errors.New("authored-script package resolution context is required")
	}
	if c == nil || c.store == nil || c.source == nil {
		return LocalArtifact{}, errors.New("authored-script package cache is not configured")
	}
	if err := validatePackageDescriptor(descriptor); err != nil {
		return LocalArtifact{}, err
	}

	artifact, err := c.store.Ensure(
		ctx,
		c.artifactKey(descriptor),
		func(ctx context.Context, destination string) error {
			return c.source.Fetch(ctx, descriptor, destination)
		},
		func(ctx context.Context, directory string) error {
			return validateArtifact(ctx, descriptor, directory)
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

func validateArtifact(ctx context.Context, descriptor Descriptor, directory string) error {
	_, err := loadDownloadedPackage(ctx, descriptor, directory)
	return err
}

func validatePackageDescriptor(descriptor Descriptor) error {
	if descriptor.Package == "" {
		return errors.New("authored-script package is required")
	}
	if descriptor.Version == "" {
		return errors.New("authored-script version is required")
	}
	if descriptor.URL == "" {
		return errors.New("authored-script package URL is required")
	}
	if descriptor.SHA256 == "" {
		return errors.New("authored-script package SHA-256 digest is required")
	}

	digest := godigest.NewDigestFromEncoded(godigest.SHA256, descriptor.SHA256)
	if err := digest.Validate(); err != nil {
		return fmt.Errorf("invalid authored-script SHA-256 digest %q: %w", descriptor.SHA256, err)
	}
	return nil
}

func artifactVariant(sourceVariant string, descriptor Descriptor) string {
	// Materialization behavior, package, and version affect validation even when
	// the source digest is the same. Including them prevents one set of
	// validation semantics from invalidating another set's cache entry.
	digest := sha256.Sum256([]byte(sourceVariant + "\x00" + descriptor.Package + "\x00" + descriptor.Version))
	return artifactKeyVersion + "-" + hex.EncodeToString(digest[:])
}

func loadDownloadedPackage(ctx context.Context, descriptor Descriptor, artifactDirectory string) (*Package, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pkg, err := LoadPackage(descriptor.Package, descriptor, LocalArtifact{Directory: artifactDirectory})
	if err != nil {
		return nil, fmt.Errorf("could not validate downloaded authored-script package: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return pkg, nil
}
