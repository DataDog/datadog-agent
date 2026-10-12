// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package oci

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/artifactstore"
)

// RegistryResolver downloads anonymous OCI artifacts without installer registry
// overrides, fallback registries, or host credential discovery.
type RegistryResolver struct {
	store     *artifactstore.Store
	transport http.RoundTripper
}

func NewRegistryResolver(directory string, client *http.Client) (*RegistryResolver, error) {
	if client == nil {
		return nil, errors.New("HTTP client is required for registry scripts")
	}
	store, err := artifactstore.New(directory)
	if err != nil {
		return nil, err
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &RegistryResolver{store: store, transport: transport}, nil
}

// Resolve authorizes the reference on every invocation, resolves mutable tags,
// and caches extracted packages by the selected platform image's digest.
func (r *RegistryResolver) Resolve(ctx context.Context, uri string, allowed []string) (*authoredscripts.Package, error) {
	ref, err := authoredscripts.AuthorizeRegistryURI(uri, allowed)
	if err != nil {
		return nil, err
	}
	descriptor, err := remote.Get(ref, remote.WithContext(ctx), remote.WithAuth(authn.Anonymous), remote.WithTransport(r.transport))
	if err != nil {
		return nil, fmt.Errorf("could not resolve script OCI reference: %w", err)
	}
	image, err := registryImage(descriptor)
	if err != nil {
		return nil, err
	}
	digest, err := image.Digest()
	if err != nil {
		return nil, err
	}
	manifest, err := image.Manifest()
	if err != nil {
		return nil, err
	}
	pkg := &fleetoci.DownloadedPackage{
		Image:   image,
		Name:    manifest.Annotations[fleetoci.AnnotationPackage],
		Version: manifest.Annotations[fleetoci.AnnotationVersion],
	}
	if pkg.Name == "" || pkg.Version == "" {
		return nil, errors.New("script OCI manifest must contain package and version annotations")
	}
	artifact, err := r.store.Ensure(ctx, artifactstore.Key{
		Namespace: pkg.Name,
		ID:        digest.Algorithm + "-" + digest.Hex,
		Variant:   materializationLayoutVersion,
	}, func(ctx context.Context, destination string) error {
		return extractPackage(ctx, pkg, destination)
	}, func(ctx context.Context, directory string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := authoredscripts.LoadRegistryPackage(pkg.Version, authoredscripts.LocalArtifact{Directory: directory})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not materialize registry script: %w", err)
	}
	return authoredscripts.LoadRegistryPackage(pkg.Version, authoredscripts.LocalArtifact{Directory: artifact.Directory})
}

func registryImage(descriptor *remote.Descriptor) (v1.Image, error) {
	if !descriptor.MediaType.IsIndex() {
		return descriptor.Image()
	}
	index, err := descriptor.ImageIndex()
	if err != nil {
		return nil, err
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	platform := v1.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	if installerenv.FromEnv().FIPSMode {
		platform.Variant = fleetoci.VariantFIPS
	}
	for _, candidate := range manifest.Manifests {
		if candidate.Platform != nil && (!candidate.Platform.Satisfies(platform) || candidate.Platform.Variant != platform.Variant) {
			continue
		}
		return index.Image(candidate.Digest)
	}
	return nil, errors.New("script OCI index has no image for this platform")
}
