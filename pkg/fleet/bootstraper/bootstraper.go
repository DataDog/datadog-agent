// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstraper provides the installer bootstraper component.
package bootstraper

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

const (
	installerPackage        = "datadog-installer"
	installerPackageVersion = "latest"
	installerBinPath        = "bin/installer/installer"
)

// Option are the options for the bootstraper.
type Option func(*options)

type options struct {
	registryAuth string
	registry     string
	apiKey       string
	site         string
}

func newOptions() *options {
	return &options{
		registryAuth: oci.RegistryAuthDefault,
		registry:     "",
		apiKey:       "",
		site:         "datadoghq.com",
	}
}

// WithRegistryAuth sets the registry authentication method.
func WithRegistryAuth(registryAuth string) Option {
	return func(o *options) {
		o.registryAuth = registryAuth
	}
}

// WithRegistry sets the registry URL.
func WithRegistry(registry string) Option {
	return func(o *options) {
		o.registry = registry
	}
}

// WithAPIKey sets the API key.
func WithAPIKey(apiKey string) Option {
	return func(o *options) {
		o.apiKey = apiKey
	}
}

// Bootstrap installs a first version of the installer on the disk.
//
// The bootstrap process is composed of the following steps:
// 1. Download the installer package from the registry.
// 2. Export the installer image as an OCI layout on the disk.
// 3. Extract the installer image layers on the disk.
// 4. Run the installer from the extract layer with `install file://<layout-path>`.
func Bootstrap(ctx context.Context, opts ...Option) error {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}

	// 1. Download the installer package from the registry.
	downloader := oci.NewDownloader(http.DefaultClient, o.registry, o.registryAuth)
	installerURL := oci.PackageURL(o.site, installerPackage, installerPackageVersion)
	downloadedPackage, err := downloader.Download(ctx, installerURL)
	if err != nil {
		return fmt.Errorf("failed to download installer package: %w", err)
	}
	if downloadedPackage.Name != installerPackage {
		return fmt.Errorf("unexpected package name: %s, expected %s", downloadedPackage.Name, installerPackage)
	}

	// 2. Export the installer image as an OCI layout on the disk.
	layoutTmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(layoutTmpDir)
	err = downloadedPackage.WriteOCILayout(layoutTmpDir)
	if err != nil {
		return fmt.Errorf("failed to write OCI layout: %w", err)
	}

	// 3. Extract the installer image layers on the disk.
	binTmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(binTmpDir)
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageLayerMediaType, binTmpDir)
	if err != nil {
		return fmt.Errorf("failed to extract layers: %w", err)
	}

	// 4. Run the installer from the extract layer with `install file://<layout-path>`.
	installerBinPath := filepath.Join(binTmpDir, installerBinPath)
	cmd := exec.NewInstallerExec(installerBinPath, o.registry, o.registryAuth, o.apiKey, o.site)
	err = cmd.Install(ctx, fmt.Sprintf("file://%s", layoutTmpDir))
	if err != nil {
		return fmt.Errorf("failed to run installer: %w", err)
	}
	return nil
}
