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

	hashFilePath = "/opt/datadog-installer/run/installer-hash"
	rootTmpDir   = "/opt/datadog-installer/run"
)

// Bootstrap installs a first version of the installer on the disk.
//
// The bootstrap process is composed of the following steps:
// 1. Download the installer package from the registry.
// 2. Export the installer image as an OCI layout on the disk.
// 3. Extract the installer image layers on the disk.
// 4. Run the installer from the extract layer with `install file://<layout-path>`.
// 5. Write a file on the disk with the hash of the installed version.
func Bootstrap(ctx context.Context, env *env.Env) error {

	// 1. Download the installer package from the registry.
	downloader := oci.NewDownloader(env, http.DefaultClient)
	version := "latest"
	if env.DefaultVersionOverrideByPackage[installerPackage] != "" {
		version = env.DefaultVersionOverrideByPackage[installerPackage]
	}
	installerURL := oci.PackageURL(env, installerPackage, version)
	downloadedPackage, err := downloader.Download(ctx, installerURL)
	if err != nil {
		return fmt.Errorf("failed to download installer package: %w", err)
	}
	if downloadedPackage.Name != installerPackage {
		return fmt.Errorf("unexpected package name: %s, expected %s", downloadedPackage.Name, installerPackage)
	}
	hash, err := downloadedPackage.Image.Digest()
	if err != nil {
		return fmt.Errorf("failed to get image digest: %w", err)
	}
	existingHash, err := os.ReadFile(hashFilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read hash file: %w", err)
	}
	if string(existingHash) == hash.String() {
		return nil
	}

	// 2. Export the installer image as an OCI layout on the disk.
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
	err = cmd.Install(ctx, fmt.Sprintf("file://%s", layoutTmpDir))
	if err != nil {
		return fmt.Errorf("failed to run installer: %w", err)
	}

	// 5. Write a file on the disk with the hash of the installed version.
	err = os.WriteFile(hashFilePath, []byte(hash.String()), 0644)
	if err != nil {
		return fmt.Errorf("failed to write hash file: %w", err)
	}
	return nil
}
