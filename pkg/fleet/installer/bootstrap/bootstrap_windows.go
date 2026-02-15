// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	iexec "github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"

	"golang.org/x/sys/windows/registry"
)

func install(ctx context.Context, env *env.Env, url string, experiment bool) error {
	err := paths.SetupInstallerDataDir()
	if err != nil {
		return fmt.Errorf("failed to create installer data directory: %w", err)
	}
	err = os.MkdirAll(paths.RootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(paths.RootTmpDir, "bootstrap")
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
	return cmd.ForceInstall(ctx, url, nil)
}

// downloadInstaller downloads the installer package from the registry and returns the path to the executable.
func downloadInstaller(ctx context.Context, env *env.Env, url string, tmpDir string) (*iexec.InstallerExec, error) {
	downloader := oci.NewDownloader(env, env.HTTPClient())
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return nil, installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	if downloadedPackage.Name != AgentPackage {
		// Only the Agent package uses the new installer each update, others use
		// the currently installed datadog-installer.exe
		return getLocalInstaller(env)
	}

	// Testing override: if InstallerBootstrapMode registry key is set, use test-specific flow
	if mode := getInstallerBootstrapMode(); mode != "" {
		return downloadInstallerTestMode(ctx, env, downloadedPackage, url, tmpDir, mode)
	}

	// Production flow: try OCI layer, fallback to MSI extraction for older packages
	installerBinPath := filepath.Join(tmpDir, "datadog-installer.exe")
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageInstallerLayerMediaType, installerBinPath) // Returns nil if the layer doesn't exist
	if err != nil {
		return nil, fmt.Errorf("failed to extract layers: %w", err)
	}
	if _, err := os.Stat(installerBinPath); err != nil {
		// Fallback to the old method if the file/layer doesn't exist
		// this is expected for versions earlier than 7.70
		return downloadInstallerOld(ctx, env, url, tmpDir)
	}
	return iexec.NewInstallerExec(env, installerBinPath), nil
}

// downloadInstallerTestMode handles bootstrap when InstallerBootstrapMode is set.
// This is ONLY used for testing to force a specific bootstrap path.
func downloadInstallerTestMode(ctx context.Context, env *env.Env, pkg *oci.DownloadedPackage, url string, tmpDir string, mode string) (*iexec.InstallerExec, error) {
	switch mode {
	case "OCI":
		// Force OCI path - fail if installer layer is missing
		installerBinPath := filepath.Join(tmpDir, "datadog-installer.exe")
		err := pkg.ExtractLayers(oci.DatadogPackageInstallerLayerMediaType, installerBinPath)
		if err != nil {
			return nil, fmt.Errorf("failed to extract installer layer: %w", err)
		}
		if _, err := os.Stat(installerBinPath); err != nil {
			return nil, fmt.Errorf("installer layer not found in OCI package (InstallerBootstrapMode=OCI): %w", err)
		}
		return iexec.NewInstallerExec(env, installerBinPath), nil
	case "MSI":
		// Force MSI fallback path
		return downloadInstallerOld(ctx, env, url, tmpDir)
	default:
		return nil, fmt.Errorf("unknown InstallerBootstrapMode: %s (expected OCI or MSI)", mode)
	}
}

// getInstallerBootstrapMode returns the bootstrap mode from registry.
// This is ONLY used for testing to force a specific bootstrap path.
// Set HKLM\SOFTWARE\Datadog\Datadog Agent\InstallerBootstrapMode to "OCI" or "MSI".
// Returns empty string if not set, which means use the default production flow.
func getInstallerBootstrapMode() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Datadog\Datadog Agent`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	val, _, err := k.GetStringValue("InstallerBootstrapMode")
	if err != nil {
		return ""
	}
	return val
}

// downloadInstallerOld downloads the installer package from the registry and returns the path to the executable.
//
// Should only be called for versions earlier than 7.70. This downloads the layer containing the MSI and then
// uses MSI admin install to extract `datadog-installer.exe` from the MSI.
func downloadInstallerOld(ctx context.Context, env *env.Env, url string, tmpDir string) (*iexec.InstallerExec, error) {
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

	layoutTmpDir, err := os.MkdirTemp(paths.RootTmpDir, "layout")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(layoutTmpDir)
	err = downloadedPackage.WriteOCILayout(layoutTmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to write OCI layout: %w", err)
	}

	err = downloadedPackage.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract layers: %w", err)
	}

	installPath, err := getInstallerPath(ctx, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get installer path: %w", err)
	}

	return iexec.NewInstallerExec(env, installPath), nil
}

func getInstallerPath(ctx context.Context, tmpDir string) (string, error) {
	installPath, msiErr := getInstallerFromMSI(ctx, tmpDir)
	if msiErr != nil {
		var err error
		installPath, err = getInstallerFromOCI(tmpDir)
		if err != nil {
			return "", fmt.Errorf("%w, %w ", err, msiErr)
		}
	}
	return installPath, nil
}

func getInstallerFromMSI(ctx context.Context, tmpDir string) (string, error) {
	msis, err := filepath.Glob(filepath.Join(tmpDir, "datadog-agent-*-x86_64.msi"))
	if err != nil {
		return "", err
	}

	if len(msis) != 1 {
		return "", fmt.Errorf("inncorect number of MSIs found %d in %s", len(msis), tmpDir)
	}

	adminInstallDir := path.Join(tmpDir, "datadog-installer")
	cmd, err := msi.Cmd(
		msi.AdministrativeInstall(),
		msi.WithMsi(msis[0]),
		msi.WithProperties(map[string]string{"TARGETDIR": strings.ReplaceAll(adminInstallDir, "/", `\`)}),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create MSI command: %w", err)
	}

	err = cmd.Run(ctx)
	if err != nil {
		err = fmt.Errorf("failed to extract Datadog Installer from the MSI: %w", err)
		var msiErr *msi.MsiexecError
		if errors.As(err, &msiErr) {
			err = fmt.Errorf("%w\n%s", err, msiErr.ProcessedLog)
		}
		return "", err
	}

	return paths.GetAdminInstallerBinaryPath(adminInstallDir), nil

}

func getInstallerFromOCI(tmpDir string) (string, error) {
	installers, err := filepath.Glob(filepath.Join(tmpDir, "datadog-installer.exe"))
	if err != nil {
		return "", err
	}
	if len(installers) == 0 {
		return "", fmt.Errorf("no installer found in %s: %w", tmpDir, fs.ErrNotExist)
	}
	return installers[0], nil
}

func getInstallerOCI(_ context.Context, env *env.Env) (string, error) {
	agentVersion := env.GetAgentVersion()
	if agentVersion != "latest" {
		ver, err := version.New(agentVersion, "")
		if err != nil {
			return "", fmt.Errorf("failed to parse agent version: %w", err)
		}
		if ver.Major < 7 || (ver.Major == 7 && ver.Minor < 65) {
			return "", fmt.Errorf("agent version %s does not support fleet automation", agentVersion)
		}
	}
	// This override is used for testing purposes
	// It allows us to specify a pipeline version to install
	if env.DefaultPackagesVersionOverride[AgentPackage] != "" {
		agentVersion = env.DefaultPackagesVersionOverride[AgentPackage]
	}
	return oci.PackageURL(env, AgentPackage, agentVersion), nil
}
