// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

func install(ctx context.Context, env *env.Env, url string, experiment bool) error {
	err := os.MkdirAll(paths.RootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(paths.RootTmpDir, "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd, err := downloadInstaller(ctx, env, url, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			err,
		)
	}
	if experiment {
		return cmd.InstallExperiment(ctx, url)
	}
	return cmd.Install(ctx, url, nil)
}

// extractInstallerFromOCI downloads the installer binary from the agent package in the registry and returns an installer executor
func downloadInstaller(ctx context.Context, env *env.Env, url string, tmpDir string) (*exec.InstallerExec, error) {
	downloader := oci.NewDownloader(env, env.HTTPClient())
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return nil, installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	if downloadedPackage.Name != AgentPackage {
		return getLocalInstaller(env)
	}

	installerBinPath := filepath.Join(tmpDir, "installer")
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageInstallerLayerMediaType, installerBinPath) // Returns nil if the layer doesn't exist
	if err != nil {
		return nil, fmt.Errorf("failed to extract layers: %w", err)
	}
	if _, err := os.Stat(installerBinPath); err != nil {
		return nil, err
	}
	// The installer.layer may be a requirefips binary that needs the OpenSSL FIPS
	// provider configured before it can start. Pass the running daemon's embedded
	// FIPS tree so the bootstrap installer finds a compatible provider.
	// Note: this points at the stable tree rather than the experiment being downloaded.
	// OpenSSL 3.x ABI stability makes this safe; see datadog-agent-finalize.rb for
	// the AT_SECURE context that requires the absolute RPATH on the binary side.
	extraEnv := fipsEnvFromRunningInstaller()
	return exec.NewInstallerExecWithExtraEnv(env, installerBinPath, extraEnv), nil
}

// fipsEnvFromRunningInstaller returns OPENSSL_CONF, OPENSSL_MODULES and
// LD_LIBRARY_PATH pointing at the running binary's embedded FIPS tree, or nil
// if the tree has no FIPS provider (non-FIPS install).
func fipsEnvFromRunningInstaller() []string {
	exePath, err := exec.GetExecutable()
	if err != nil {
		return nil
	}
	// <install>/embedded/bin/installer → <install>/embedded
	embedded := filepath.Dir(filepath.Dir(exePath))
	return env.FIPSProviderEnv(embedded)
}

func getInstallerOCI(_ context.Context, env *env.Env) (string, error) {
	version := "latest"
	if env.DefaultPackagesVersionOverride[InstallerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[InstallerPackage]
	}
	return oci.PackageURL(env, InstallerPackage, version), nil
}
