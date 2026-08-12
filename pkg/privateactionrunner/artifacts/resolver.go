// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifacts

import (
	"context"
	"errors"
	"fmt"
)

// Resolver finds an authorized artifact and makes it available locally.
type Resolver struct {
	catalog  Catalog
	provider Provider
}

func NewResolver(catalog Catalog, provider Provider) *Resolver {
	return &Resolver{
		catalog:  catalog,
		provider: provider,
	}
}

func (r *Resolver) Resolve(ctx context.Context, key string, platform Platform) (Descriptor, LocalArtifact, error) {
	if ctx == nil {
		return Descriptor{}, LocalArtifact{}, errors.New("artifact resolution context is required")
	}
	if r == nil || r.catalog == nil || r.provider == nil {
		return Descriptor{}, LocalArtifact{}, errors.New("artifact resolver is not configured")
	}

	descriptor, err := r.catalog.Lookup(key, platform)
	if err != nil {
		return Descriptor{}, LocalArtifact{}, fmt.Errorf("could not resolve artifact %q: %w", key, err)
	}
	if err := validateDescriptor(descriptor, platform); err != nil {
		return Descriptor{}, LocalArtifact{}, fmt.Errorf("artifact catalog returned an invalid descriptor for %q: %w", key, err)
	}

	artifact, err := r.provider.Get(ctx, descriptor)
	if err != nil {
		return Descriptor{}, LocalArtifact{}, fmt.Errorf("could not make artifact %q available locally: %w", key, err)
	}

	return descriptor, artifact, nil
}

func validateDescriptor(descriptor Descriptor, platform Platform) error {
	if descriptor.Name == "" {
		return errors.New("artifact name is required")
	}
	if descriptor.Version == "" {
		return errors.New("artifact version is required")
	}
	if descriptor.Digest == "" {
		return errors.New("artifact digest is required")
	}
	if descriptor.Platform != platform {
		return fmt.Errorf("artifact platform %s/%s does not match requested platform %s/%s", descriptor.Platform.OS, descriptor.Platform.Arch, platform.OS, platform.Arch)
	}
	return nil
}
