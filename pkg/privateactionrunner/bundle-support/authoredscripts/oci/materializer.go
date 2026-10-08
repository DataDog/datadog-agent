// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package oci provides an OCI-backed authored-script package materializer.
package oci

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"runtime"
	"strings"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

const (
	materializationLayoutVersion          = "authored-script-layout-v1"
	datadogPackageExtensionNameAnnotation = "com.datadoghq.package.extension.name"
)

// Materializer materializes authored-script packages from OCI images.
type Materializer struct {
	downloader        *fleetoci.Downloader
	materializationID string
}

// NewMaterializer creates an OCI package materializer using the Fleet downloader.
func NewMaterializer(environment *installerenv.Env, client *http.Client) (*Materializer, error) {
	if environment == nil {
		return nil, errors.New("installer environment is required for authored-script OCI downloads")
	}
	if client == nil {
		return nil, errors.New("HTTP client is required for authored-script OCI downloads")
	}

	environmentCopy := *environment
	environmentCopy.RegistryOverrideByImage = maps.Clone(environment.RegistryOverrideByImage)
	environmentCopy.RegistryAuthOverrideByImage = maps.Clone(environment.RegistryAuthOverrideByImage)
	environmentCopy.RegistryUsernameByImage = maps.Clone(environment.RegistryUsernameByImage)
	environmentCopy.RegistryPasswordByImage = maps.Clone(environment.RegistryPasswordByImage)
	flavor := "base"
	if environmentCopy.FIPSMode {
		flavor = fleetoci.VariantFIPS
	}
	return &Materializer{
		downloader: fleetoci.NewDownloader(&environmentCopy, client),
		materializationID: strings.Join([]string{
			materializationLayoutVersion,
			runtime.GOOS,
			runtime.GOARCH,
			flavor,
		}, "-"),
	}, nil
}

// MaterializationID identifies the platform, flavor, and extracted layout.
func (m *Materializer) MaterializationID() string {
	return m.materializationID
}

// Materialize downloads an authored-script package and extracts its OCI main
// layer as the script and each OCI extension layer as a bundled dependency.
func (m *Materializer) Materialize(ctx context.Context, descriptor authoredscripts.Descriptor, destination string) error {
	if destination == "" {
		return errors.New("authored-script OCI destination is required")
	}

	downloadedPackage, err := m.downloader.Download(ctx, descriptor.URL)
	if err != nil {
		return fmt.Errorf("could not download authored-script OCI package: %w", err)
	}
	if downloadedPackage.Name != descriptor.Package {
		return fmt.Errorf("OCI package name %q does not match catalog package %q", downloadedPackage.Name, descriptor.Package)
	}
	if downloadedPackage.Version != descriptor.Version {
		return fmt.Errorf("OCI package version %q does not match catalog version %q", downloadedPackage.Version, descriptor.Version)
	}

	dependencyNames, err := dependencyLayerNames(downloadedPackage)
	if err != nil {
		return err
	}
	artifact := authoredscripts.LocalArtifact{Directory: destination}
	if err := extractScriptLayer(ctx, downloadedPackage, artifact.ScriptDirectory()); err != nil {
		return fmt.Errorf("could not extract authored-script OCI package layer: %w", err)
	}
	for _, name := range dependencyNames {
		if err := extractDependencyLayer(ctx, downloadedPackage, name, artifact.DependencyDirectory(name)); err != nil {
			return fmt.Errorf("could not extract authored-script dependency %q: %w", name, err)
		}
	}
	return nil
}

func dependencyLayerNames(downloadedPackage *fleetoci.DownloadedPackage) ([]string, error) {
	imageManifest, err := downloadedPackage.Image.Manifest()
	if err != nil {
		return nil, fmt.Errorf("could not inspect authored-script OCI package layers: %w", err)
	}
	mainLayerCount := 0
	dependencyNames := make([]string, 0)
	seenDependencyNames := make(map[string]struct{})
	for _, layer := range imageManifest.Layers {
		switch layer.MediaType {
		case fleetoci.DatadogPackageLayerMediaType:
			mainLayerCount++
		case fleetoci.DatadogPackageExtensionLayerMediaType:
			name := layer.Annotations[datadogPackageExtensionNameAnnotation]
			if err := authoredscripts.ValidateDependencyName(name); err != nil {
				return nil, fmt.Errorf("invalid authored-script OCI dependency layer: %w", err)
			}
			if _, found := seenDependencyNames[name]; found {
				return nil, fmt.Errorf("authored-script OCI package contains duplicate dependency %q", name)
			}
			seenDependencyNames[name] = struct{}{}
			dependencyNames = append(dependencyNames, name)
		}
	}
	if mainLayerCount != 1 {
		return nil, fmt.Errorf("authored-script OCI package must contain exactly one main layer, found %d", mainLayerCount)
	}
	return dependencyNames, nil
}

func extractScriptLayer(ctx context.Context, downloadedPackage *fleetoci.DownloadedPackage, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("could not create layer directory: %w", err)
	}
	return downloadedPackage.ExtractLayers(ctx, fleetoci.DatadogPackageLayerMediaType, destination)
}

func extractDependencyLayer(ctx context.Context, downloadedPackage *fleetoci.DownloadedPackage, name, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("could not create layer directory: %w", err)
	}
	return downloadedPackage.ExtractLayers(
		ctx,
		fleetoci.DatadogPackageExtensionLayerMediaType,
		destination,
		fleetoci.LayerAnnotation{Key: datadogPackageExtensionNameAnnotation, Value: name},
	)
}
