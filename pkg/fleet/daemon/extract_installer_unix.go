// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

// extractInstallerFromOCI downloads the installer binary from the agent package in the registry and returns an installer executor
func extractInstallerFromOCI(ctx context.Context, env *env.Env, url string, tmpDir string) (*exec.InstallerExec, error) {
	downloader := oci.NewDownloader(env, env.HTTPClient())
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to download installer package: %w", err)
	}

	installerBinPath := filepath.Join(tmpDir, "installer")
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageInstallerLayerMediaType, installerBinPath) // Returns nil if the layer doesn't exist
	if err != nil {
		return nil, fmt.Errorf("failed to extract layers: %w", err)
	}

	// Backwards compatibility: if the installer binary is not found in the expected path, return ourselves
	if _, err := os.Stat(installerBinPath); err != nil {
		if os.IsNotExist(err) {
			installerBin, err := os.Executable()
			if err != nil {
				return nil, fmt.Errorf("could not get installer executable path: %w", err)
			}
			installerBinPath, err = filepath.EvalSymlinks(installerBin)
			if err != nil {
				return nil, fmt.Errorf("could not get resolve installer executable path: %w", err)
			}
		}
	}
	return exec.NewInstallerExec(env, installerBinPath), nil
}
