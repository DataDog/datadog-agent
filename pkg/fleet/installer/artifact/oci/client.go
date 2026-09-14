// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package oci provides an additive, descriptor-based view of Datadog Package
// OCI artifacts. It uses the existing Fleet downloader for registry access and
// platform selection without changing Fleet's APIs or extraction behavior.
package oci

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	ociV1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact/safetar"
	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

const (
	// AnnotationExtensionName identifies a named Datadog Package extension layer.
	AnnotationExtensionName = "com.datadoghq.package.extension.name"
)

// Layer describes one layer in the selected platform image.
type Layer struct {
	Digest      digest.Digest
	MediaType   types.MediaType
	Size        int64
	Annotations map[string]string
}

// Package is a downloaded and metadata-validated platform image.
type Package struct {
	descriptor artifact.Descriptor
	downloaded *fleetoci.DownloadedPackage
	layers     []Layer
}

// Descriptor returns the immutable catalog descriptor used for this package.
func (p *Package) Descriptor() artifact.Descriptor { return p.descriptor }

// Layers returns a copy of the selected image's layer metadata.
func (p *Package) Layers() []Layer {
	if p == nil {
		return nil
	}
	layers := make([]Layer, len(p.layers))
	for index, layer := range p.layers {
		layer.Annotations = maps.Clone(layer.Annotations)
		layers[index] = layer
	}
	return layers
}

// LayersByMediaType returns layers of mediaType in manifest order.
func (p *Package) LayersByMediaType(mediaType types.MediaType) []Layer {
	if p == nil {
		return nil
	}
	return slices.DeleteFunc(p.Layers(), func(layer Layer) bool { return layer.MediaType != mediaType })
}

// ExtractLayer strictly extracts one package layer into destination. The
// destination must be a newly created, dedicated directory.
func (p *Package) ExtractLayer(ctx context.Context, selected Layer, destination string, extractor safetar.Extractor, maxCompressedBytes int64) error {
	if ctx == nil {
		return errors.New("OCI layer extraction context is required")
	}
	if p == nil || p.downloaded == nil {
		return errors.New("downloaded OCI package is required")
	}
	if maxCompressedBytes <= 0 {
		return errors.New("maximum compressed layer size must be positive")
	}
	if selected.Size < 0 || selected.Size > maxCompressedBytes {
		return fmt.Errorf("OCI layer %q compressed size %d exceeds the %d byte limit", selected.Digest, selected.Size, maxCompressedBytes)
	}
	manifestLayer, found, err := p.findLayer(selected)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("OCI layer %q is not part of package %q", selected.Digest, p.descriptor.Package)
	}
	if destination == "" || !filepath.IsAbs(destination) {
		return errors.New("OCI layer destination must be an absolute path")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return fmt.Errorf("could not create OCI layer destination: %w", err)
	}
	imageLayer, err := p.downloaded.Image.LayerByDigest(manifestLayer.Digest)
	if err != nil {
		return fmt.Errorf("could not open OCI layer %q: %w", selected.Digest, err)
	}
	reader, err := imageLayer.Uncompressed()
	if err != nil {
		return fmt.Errorf("could not read OCI layer %q: %w", selected.Digest, err)
	}
	defer reader.Close()
	if err := extractor.Extract(ctx, reader, destination); err != nil {
		return fmt.Errorf("could not extract OCI layer %q: %w", selected.Digest, err)
	}
	return nil
}

func (p *Package) findLayer(selected Layer) (ociV1.Descriptor, bool, error) {
	manifest, err := p.downloaded.Image.Manifest()
	if err != nil {
		return ociV1.Descriptor{}, false, fmt.Errorf("could not read OCI package manifest: %w", err)
	}
	for _, layer := range manifest.Layers {
		layerDigest, err := digest.Parse(layer.Digest.String())
		if err != nil {
			return ociV1.Descriptor{}, false, fmt.Errorf("invalid OCI layer digest %q: %w", layer.Digest, err)
		}
		if layerDigest == selected.Digest && layer.MediaType == selected.MediaType && layer.Size == selected.Size && maps.Equal(layer.Annotations, selected.Annotations) {
			return layer, true, nil
		}
	}
	return ociV1.Descriptor{}, false, nil
}

// Client downloads immutable OCI packages for the current host platform.
type Client struct {
	downloader *fleetoci.Downloader
	platform   artifact.Platform
}

// NewClient creates a Client without modifying the supplied installer
// environment. The existing Fleet downloader remains the owner of registry,
// mirror, authentication, and platform-selection behavior.
func NewClient(environment *installerenv.Env, httpClient *http.Client) (*Client, error) {
	if environment == nil {
		return nil, errors.New("installer environment is required for OCI artifacts")
	}
	if httpClient == nil {
		return nil, errors.New("HTTP client is required for OCI artifacts")
	}
	environmentCopy := *environment
	environmentCopy.RegistryOverrideByImage = maps.Clone(environment.RegistryOverrideByImage)
	environmentCopy.RegistryAuthOverrideByImage = maps.Clone(environment.RegistryAuthOverrideByImage)
	environmentCopy.RegistryUsernameByImage = maps.Clone(environment.RegistryUsernameByImage)
	environmentCopy.RegistryPasswordByImage = maps.Clone(environment.RegistryPasswordByImage)
	platform := artifact.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	if environmentCopy.FIPSMode {
		platform.Variant = fleetoci.VariantFIPS
	}
	return &Client{downloader: fleetoci.NewDownloader(&environmentCopy, httpClient), platform: platform}, nil
}

// Platform returns the exact host platform selected by this Client.
func (c *Client) Platform() artifact.Platform {
	if c == nil {
		return artifact.Platform{}
	}
	return c.platform
}

// Download downloads and validates descriptor's package metadata and layers.
func (c *Client) Download(ctx context.Context, descriptor artifact.Descriptor) (*Package, error) {
	if ctx == nil {
		return nil, errors.New("OCI artifact context is required")
	}
	if c == nil || c.downloader == nil {
		return nil, errors.New("OCI artifact client is not configured")
	}
	if err := validateDescriptor(descriptor, c.platform); err != nil {
		return nil, err
	}
	downloaded, err := c.downloader.Download(ctx, descriptor.Reference)
	if err != nil {
		return nil, fmt.Errorf("could not download OCI artifact: %w", err)
	}
	if downloaded.Name != descriptor.Package {
		return nil, fmt.Errorf("OCI package name %q does not match catalog package %q", downloaded.Name, descriptor.Package)
	}
	if downloaded.Version != descriptor.Version {
		return nil, fmt.Errorf("OCI package version %q does not match catalog version %q", downloaded.Version, descriptor.Version)
	}
	manifest, err := downloaded.Image.Manifest()
	if err != nil {
		return nil, fmt.Errorf("could not read OCI package manifest: %w", err)
	}
	layers := make([]Layer, 0, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		layerDigest, err := digest.Parse(layer.Digest.String())
		if err != nil {
			return nil, fmt.Errorf("invalid OCI layer digest %q: %w", layer.Digest, err)
		}
		layers = append(layers, Layer{
			Digest:      layerDigest,
			MediaType:   layer.MediaType,
			Size:        layer.Size,
			Annotations: maps.Clone(layer.Annotations),
		})
	}
	return &Package{descriptor: descriptor, downloaded: downloaded, layers: layers}, nil
}

func validateDescriptor(descriptor artifact.Descriptor, selectedPlatform artifact.Platform) error {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if descriptor.Platform.OS != "" && (descriptor.Platform.OS != selectedPlatform.OS || descriptor.Platform.Architecture != selectedPlatform.Architecture) {
		return fmt.Errorf("artifact platform %s/%s does not match selected platform %s/%s", descriptor.Platform.OS, descriptor.Platform.Architecture, selectedPlatform.OS, selectedPlatform.Architecture)
	}
	if descriptor.Platform.Variant != "" && descriptor.Platform.Variant != selectedPlatform.Variant {
		return fmt.Errorf("artifact platform variant %q does not match selected variant %q", descriptor.Platform.Variant, selectedPlatform.Variant)
	}
	parsedURL, err := url.Parse(descriptor.Reference)
	if err != nil {
		return fmt.Errorf("could not parse OCI artifact reference: %w", err)
	}
	if parsedURL.Scheme != "oci" || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return errors.New("OCI artifact reference must be an OCI digest URL without user information, a query, or a fragment")
	}
	reference, err := name.NewDigest(strings.TrimPrefix(descriptor.Reference, "oci://"), name.StrictValidation)
	if err != nil {
		return fmt.Errorf("OCI artifact reference must contain an immutable digest: %w", err)
	}
	if reference.DigestStr() != descriptor.Digest.String() {
		return fmt.Errorf("OCI artifact reference digest %q does not match descriptor digest %q", reference.DigestStr(), descriptor.Digest)
	}
	return nil
}
