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

// MaterializedPackage describes the package metadata observed by a
// PackageMaterializer while materializing an artifact.
type MaterializedPackage struct {
	Package string
	Version string
}

// PackageMaterializer downloads and materializes one authored-script package
// into destination. Implementations own source-specific work such as selecting
// and extracting OCI layers. They must not write outside destination.
//
// CacheVariant must identify every materialization choice not represented by
// Descriptor.SHA256, including platform, flavor, and output-layout version.
// Both methods must be stable across processes, and Materialize must be safe to
// call concurrently for different packages.
type PackageMaterializer interface {
	CacheVariant() string
	Materialize(ctx context.Context, descriptor Descriptor, destination string) (MaterializedPackage, error)
}

// Downloader safely downloads and caches authored-script packages. It is safe
// for concurrent use. It does not perform catalog lookup or decide whether a
// package is authorized.
type Downloader struct {
	store        *artifactstore.Store
	materializer PackageMaterializer
	cacheVariant string
}

// NewDownloader creates an authored-script downloader. rootDirectory must be a
// dedicated, trusted local-filesystem directory; see artifactstore.New for the
// complete ownership and filesystem requirements.
func NewDownloader(rootDirectory string, materializer PackageMaterializer) (*Downloader, error) {
	if materializer == nil {
		return nil, errors.New("authored-script package materializer is required")
	}
	cacheVariant := materializer.CacheVariant()
	if cacheVariant == "" {
		return nil, errors.New("authored-script package materializer cache variant is required")
	}
	store, err := artifactstore.New(rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not create authored-script artifact store: %w", err)
	}
	return &Downloader{
		store:        store,
		materializer: materializer,
		cacheVariant: cacheVariant,
	}, nil
}

// NewUserCacheDownloader creates a downloader rooted in the current user's
// authored-script cache directory.
func NewUserCacheDownloader(materializer PackageMaterializer) (*Downloader, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	return NewDownloader(
		filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptCacheDirectory),
		materializer,
	)
}

// Download returns a validated local artifact for descriptor, downloading and
// atomically publishing it on a cache miss. Descriptor is assumed to have been
// authorized by an upstream catalog boundary.
func (d *Downloader) Download(ctx context.Context, descriptor Descriptor) (LocalArtifact, error) {
	if ctx == nil {
		return LocalArtifact{}, errors.New("authored-script download context is required")
	}
	if d == nil || d.store == nil || d.materializer == nil {
		return LocalArtifact{}, errors.New("authored-script downloader is not configured")
	}
	if err := validateDownloadDescriptor(descriptor); err != nil {
		return LocalArtifact{}, err
	}

	key := artifactstore.Key{
		Namespace: artifactDigestNamespace,
		ID:        descriptor.SHA256,
		Variant:   artifactKeyVersion + "-" + descriptorValidationID(d.cacheVariant, descriptor),
	}
	artifact, err := d.store.Ensure(
		ctx,
		key,
		func(ctx context.Context, destination string) error {
			metadata, err := d.materializer.Materialize(ctx, descriptor, destination)
			if err != nil {
				return err
			}
			pkg, err := loadDownloadedPackage(ctx, descriptor, destination)
			if err != nil {
				return err
			}
			if metadata.Package != pkg.Manifest.Package {
				return fmt.Errorf("materialized package name %q does not match authored-script manifest package %q", metadata.Package, pkg.Manifest.Package)
			}
			if metadata.Version != pkg.Manifest.Version {
				return fmt.Errorf("materialized package version %q does not match authored-script manifest version %q", metadata.Version, pkg.Manifest.Version)
			}
			return nil
		},
		func(ctx context.Context, artifactDirectory string) error {
			_, err := loadDownloadedPackage(ctx, descriptor, artifactDirectory)
			return err
		},
	)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not ensure authored-script package %q version %q: %w", descriptor.Package, descriptor.Version, err)
	}
	return LocalArtifact{Directory: artifact.Directory}, nil
}

func validateDownloadDescriptor(descriptor Descriptor) error {
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

func descriptorValidationID(cacheVariant string, descriptor Descriptor) string {
	// Materialization behavior, package, and version affect validation even when
	// the source digest is the same. Including them prevents one set of
	// validation semantics from invalidating another set's cache entry.
	digest := sha256.Sum256([]byte(cacheVariant + "\x00" + descriptor.Package + "\x00" + descriptor.Version))
	return hex.EncodeToString(digest[:])
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
