// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package artifact provides content-addressed materialization of immutable
// artifacts.
//
// The package deliberately does not perform catalog lookup or decide whether
// an artifact is authorized. Callers must resolve a Descriptor at their trust
// boundary and, when required, re-authorize it before using the returned
// Handle.
package artifact

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/opencontainers/go-digest"
)

// Platform identifies the artifact variant selected for the current host.
type Platform struct {
	OS           string
	Architecture string
	Variant      string
}

// Descriptor identifies one immutable artifact published by a catalog.
// Reference locates the source bytes, while Digest provides their immutable
// identity. Source adapters may impose a stronger reference contract; for
// example, the OCI client requires a digest URL that agrees with Digest.
type Descriptor struct {
	Package   string
	Version   string
	Reference string
	Digest    digest.Digest
	Size      int64 // Optional catalog-provided size metadata; zero means unspecified.
	Platform  Platform
}

// Validate checks the source-independent descriptor fields.
func (d Descriptor) Validate() error {
	if d.Package == "" {
		return errors.New("artifact package is required")
	}
	if d.Version == "" {
		return errors.New("artifact version is required")
	}
	if d.Reference == "" {
		return errors.New("artifact reference is required")
	}
	if d.Digest == "" {
		return errors.New("artifact digest is required")
	}
	if err := d.Digest.Validate(); err != nil {
		return fmt.Errorf("invalid artifact digest %q: %w", d.Digest, err)
	}
	if d.Digest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("unsupported artifact digest algorithm %q", d.Digest.Algorithm())
	}
	if d.Size < 0 {
		return errors.New("artifact size cannot be negative")
	}
	if (d.Platform.OS == "") != (d.Platform.Architecture == "") {
		return errors.New("artifact platform OS and architecture must be set together")
	}
	if d.Platform.Variant != "" && d.Platform.OS == "" {
		return errors.New("artifact platform variant requires an OS and architecture")
	}
	return nil
}

// Handle identifies a fully materialized artifact directory.
type Handle struct {
	Descriptor Descriptor
	Directory  string
}

// Materializer defines the source and on-disk representation of an artifact.
// ID must change whenever the materialized layout or its validation semantics
// change. Materialize must authenticate source bytes against Descriptor.Digest
// and write only below destination. Validate does not modify the artifact and
// must be safe for concurrent use across processes.
type Materializer interface {
	ID() string
	Materialize(context.Context, Descriptor, string) error
	Validate(context.Context, Descriptor, string) error
}

// Manager coordinates materialization through a Store.
type Manager struct {
	store        *Store
	materializer Materializer
}

// NewManager creates a Manager rooted at root. root must be a dedicated,
// trusted local-filesystem directory; see NewStore for the complete contract.
func NewManager(root string, materializer Materializer) (*Manager, error) {
	if materializer == nil {
		return nil, errors.New("artifact materializer is required")
	}
	if materializer.ID() == "" {
		return nil, errors.New("artifact materializer ID is required")
	}
	store, err := NewStore(root)
	if err != nil {
		return nil, err
	}
	return &Manager{store: store, materializer: materializer}, nil
}

// Ensure returns a validated cached artifact or materializes and atomically
// publishes it. Ensure does not imply that descriptor remains authorized after
// it returns.
func (m *Manager) Ensure(ctx context.Context, descriptor Descriptor) (Handle, error) {
	if ctx == nil {
		return Handle{}, errors.New("artifact context is required")
	}
	if m == nil || m.store == nil || m.materializer == nil {
		return Handle{}, errors.New("artifact manager is not configured")
	}
	if err := descriptor.Validate(); err != nil {
		return Handle{}, err
	}

	key := newKey(descriptor, m.materializer.ID())
	stored, err := m.store.Ensure(
		ctx,
		key,
		func(ctx context.Context, destination string) error {
			return m.materializer.Materialize(ctx, descriptor, destination)
		},
		func(ctx context.Context, directory string) error {
			return m.materializer.Validate(ctx, descriptor, directory)
		},
	)
	if err != nil {
		return Handle{}, fmt.Errorf("could not materialize package %q version %q: %w", descriptor.Package, descriptor.Version, err)
	}
	return Handle{Descriptor: descriptor, Directory: filepath.Clean(stored.Directory)}, nil
}
