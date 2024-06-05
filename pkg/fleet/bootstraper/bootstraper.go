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

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

const (
	installerPackage = "datadog-installer"
	installerBinPath = "bin/installer/installer"

	rootTmpDir = "/opt/datadog-installer/tmp"
)

// Bootstrap installs a first version of the installer on the disk.
//
// The bootstrap process is composed of the following steps:
// 1. Download the installer package from the registry.
// 2. Export the installer image as an OCI layout on the disk.
// 3. Extract the installer image layers on the disk.
// 4. Run the installer from the extract layer with `install file://<layout-path>`.
// 5. Get the list of default packages to install and install them.
func Bootstrap(ctx context.Context, env *env.Env) error {
	// 1. Download the installer package from the registry.
	downloader := oci.NewDownloader(env, http.DefaultClient)
	version := "latest"
	if env.DefaultPackagesVersionOverride[installerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[installerPackage]
	}
	installerURL := oci.PackageURL(env, installerPackage, version)
	downloadedPackage, err := downloader.Download(ctx, installerURL)
	if err != nil {
		return fmt.Errorf("failed to download installer package: %w", err)
	}
	if downloadedPackage.Name != installerPackage {
		return fmt.Errorf("unexpected package name: %s, expected %s", downloadedPackage.Name, installerPackage)
	}

	// 2. Export the installer image as an OCI layout on the disk.
	err = os.MkdirAll(rootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	layoutTmpDir, err := os.MkdirTemp(rootTmpDir, "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(layoutTmpDir)
	err = downloadedPackage.WriteOCILayout(layoutTmpDir)
	if err != nil {
		return fmt.Errorf("failed to write OCI layout: %w", err)
	}

	// 3. Extract the installer image layers on the disk.
	binTmpDir, err := os.MkdirTemp(rootTmpDir, "")
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
	cmd := exec.NewInstallerExec(env, installerBinPath)
	err = cmd.Install(ctx, fmt.Sprintf("file://%s", layoutTmpDir), nil)
	if err != nil {
		return fmt.Errorf("failed to run installer: %w", err)
	}

	// 6. Get the list of default packages to install and install them.
	defaultPackages, err := cmd.DefaultPackages(ctx)
	if err != nil {
		return fmt.Errorf("failed to get default packages: %w", err)
	}
	for _, url := range defaultPackages {
		err = cmd.Install(ctx, url, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to install package %s: %v\n", url, err)
		}
	}
	return nil
}
