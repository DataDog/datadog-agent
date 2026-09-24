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
	"runtime"
	"strings"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

const materializationLayoutVersion = "datadog-package-v1"

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

// Materialize downloads and extracts the main Datadog Package layer.
func (m *Materializer) Materialize(ctx context.Context, descriptor authoredscripts.Descriptor, destination string) error {
	if destination == "" {
		return errors.New("authored-script OCI destination is required")
	}

	downloadedPackage, err := m.downloader.Download(ctx, descriptor.URL)
	if err != nil {
		return fmt.Errorf("could not download authored-script OCI package: %w", err)
	}
	if downloadedPackage == nil {
		return errors.New("authored-script OCI downloader returned no package")
	}
	if downloadedPackage.Name != descriptor.Package {
		return fmt.Errorf("OCI package name %q does not match catalog package %q", downloadedPackage.Name, descriptor.Package)
	}
	if downloadedPackage.Version != descriptor.Version {
		return fmt.Errorf("OCI package version %q does not match catalog version %q", downloadedPackage.Version, descriptor.Version)
	}
	if err := downloadedPackage.ExtractLayers(ctx, fleetoci.DatadogPackageLayerMediaType, destination); err != nil {
		return fmt.Errorf("could not extract authored-script OCI package: %w", err)
	}
	return nil
}
