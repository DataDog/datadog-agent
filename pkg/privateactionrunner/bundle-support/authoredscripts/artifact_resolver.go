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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/artifactstore"
)

const (
	artifactDigestIDPrefix       = "sha256-"
	datadogAgentCacheDirectory   = "datadog-agent"
	authoredScriptCacheDirectory = "dd-authored-script"
)

// LocalArtifact identifies an immutable artifact directory that is ready for use.
type LocalArtifact = artifactstore.Artifact

// PackageMaterializer materializes an authored-script package. MaterializationID must
// identify every materialization choice not represented by the package digest.
type PackageMaterializer interface {
	MaterializationID() string
	Materialize(ctx context.Context, descriptor Descriptor, destination string) error
}

// ArtifactResolver resolves descriptors to validated local artifacts.
type ArtifactResolver struct {
	store             *artifactstore.Store
	materializer      PackageMaterializer
	materializationID string
}

// NewArtifactResolver creates an authored-script artifact resolver.
func NewArtifactResolver(rootDirectory string, materializer PackageMaterializer) (*ArtifactResolver, error) {
	if materializer == nil {
		return nil, errors.New("authored-script package materializer is required")
	}
	materializationID := materializer.MaterializationID()
	if materializationID == "" {
		return nil, errors.New("authored-script package materialization ID is required")
	}
	store, err := artifactstore.New(rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not create authored-script artifact store: %w", err)
	}
	return &ArtifactResolver{
		store:             store,
		materializer:      materializer,
		materializationID: materializationID,
	}, nil
}

// NewUserArtifactResolver creates an artifact resolver rooted in the current user's
// authored-script cache directory.
func NewUserArtifactResolver(materializer PackageMaterializer) (*ArtifactResolver, error) {
	userCacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not locate the OS user cache: %w", err)
	}
	return NewArtifactResolver(
		filepath.Join(userCacheDirectory, datadogAgentCacheDirectory, authoredScriptCacheDirectory),
		materializer,
	)
}

// Resolve returns a validated local artifact, downloading and publishing it on
// a cache miss. The descriptor must already be authorized by the catalog.
func (r *ArtifactResolver) Resolve(ctx context.Context, descriptor Descriptor) (LocalArtifact, error) {
	if err := descriptor.Validate(); err != nil {
		return LocalArtifact{}, err
	}

	artifact, err := r.store.Ensure(
		ctx,
		r.artifactKey(descriptor),
		func(ctx context.Context, destination string) error {
			return r.materializer.Materialize(ctx, descriptor, destination)
		},
		func(ctx context.Context, directory string) error {
			return validateCachedPackage(ctx, descriptor, directory)
		},
	)
	if err != nil {
		return LocalArtifact{}, fmt.Errorf("could not ensure authored-script package %q version %q: %w", descriptor.Package, descriptor.Version, err)
	}
	return artifact, nil
}

func (r *ArtifactResolver) artifactKey(descriptor Descriptor) artifactstore.Key {
	return artifactstore.Key{
		Namespace: descriptor.Package,
		ID:        artifactDigestIDPrefix + descriptor.SHA256,
		Variant:   r.materializationID,
	}
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
