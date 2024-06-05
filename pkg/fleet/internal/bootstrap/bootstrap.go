// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

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

// Install self-installs the installer package from the given URL.
func Install(ctx context.Context, env *env.Env, url string) error {
	err := os.MkdirAll(rootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(rootTmpDir, "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd, err := downloadInstaller(ctx, env, url, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to download installer: %w", err)
	}
	return cmd.Install(ctx, url, nil)
}

// InstallExperiment self-installs the installer package from the given URL as an experiment.
func InstallExperiment(ctx context.Context, env *env.Env, url string) error {
	err := os.MkdirAll(rootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(rootTmpDir, "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd, err := downloadInstaller(ctx, env, url, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to download installer: %w", err)
	}
	return cmd.InstallExperiment(ctx, url)
}

// downloadInstaller downloads the installer package from the registry and returns an installer executor.
//
// This process is made to have the least assumption possible as it is long lived and should always work in the future.
// 1. Download the installer package from the registry.
// 2. Export the installer image as an OCI layout on the disk.
// 3. Extract the installer image layers on the disk.
// 4. Create an installer executor from the extract layer.
func downloadInstaller(ctx context.Context, env *env.Env, url string, tmpDir string) (*exec.InstallerExec, error) {
	// 1. Download the installer package from the registry.
	downloader := oci.NewDownloader(env, http.DefaultClient)
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to download installer package: %w", err)
	}
	if downloadedPackage.Name != installerPackage {
		return nil, fmt.Errorf("unexpected package name: %s, expected %s", downloadedPackage.Name, installerPackage)
	}

	// 2. Export the installer image as an OCI layout on the disk.
	layoutTmpDir, err := os.MkdirTemp(rootTmpDir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(layoutTmpDir)
	err = downloadedPackage.WriteOCILayout(layoutTmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to write OCI layout: %w", err)
	}

	// 3. Extract the installer image layers on the disk.
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract layers: %w", err)
	}

	// 4. Create an installer executor from the extract layer.
	installerBinPath := filepath.Join(tmpDir, installerBinPath)
	return exec.NewInstallerExec(env, installerBinPath), nil
}
