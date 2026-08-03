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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"

	"golang.org/x/sys/windows/registry"
)

func install(ctx context.Context, env *env.Env, url string, experiment bool) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "bootstrap.install")
	defer func() { span.Finish(err) }()
	span.SetTag("url", url)
	span.SetTag("experiment", experiment)
	err = paths.SetupInstallerDataDir()
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
//   - For non-Agent OCI packages: returns the locally-installed datadog-installer.exe
//     (other Datadog packages do not ship a per-version installer.exe).
//   - For the Agent package: extracts a version-matched datadog-installer.exe from the OCI package.
func downloadInstaller(ctx context.Context, env *env.Env, url string, tmpDir string) (_ *iexec.InstallerExec, err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "bootstrap.download_installer")
	defer func() { span.Finish(err) }()
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
	installerBinPath, err := extractInstallerFromAgentPackage(ctx, downloadedPackage, tmpDir)
	if err != nil {
		return nil, err
	}
	return iexec.NewInstallerExec(env, installerBinPath), nil
}

// DownloadInstallerExe downloads the Datadog Agent OCI package at url and
// returns a local-disk path to a version-matched datadog-installer.exe.
// Resolution order:
//
//   - dedicated datadog-installer.exe OCI layer
//     (DatadogPackageInstallerLayerMediaType, Agent 7.79+);
//   - MSI admin-install extraction when the OCI installer layer is absent (Agent 7.78 and earlier);
//   - find datadog-installer.exe in the extracted OCI package, as a fallback when MSI extraction fails.
//
// Honors the InstallerBootstrapMode registry key (`OCI` / `MSI`) for
// testing. The url must point to the Agent OCI package; passing any
// other package returns an error.
func DownloadInstallerExe(ctx context.Context, env *env.Env, url string, tmpDir string) (string, error) {
	downloader := oci.NewDownloader(env, env.HTTPClient())
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return "", installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	if downloadedPackage.Name != AgentPackage {
		return "", fmt.Errorf("expected %s OCI package, got %s", AgentPackage, downloadedPackage.Name)
	}
	return extractInstallerFromAgentPackage(ctx, downloadedPackage, tmpDir)
}

// extractInstallerFromAgentPackage extracts a version-matched
// datadog-installer.exe from an already-downloaded Agent OCI package.
//
// Production flow: try the dedicated installer.exe layer first, fall
// back to MSI admin-install extraction for older packages.
// Honors the InstallerBootstrapMode registry key (`OCI` / `MSI`) to
// force one path or the other for testing.
func extractInstallerFromAgentPackage(ctx context.Context, pkg *oci.DownloadedPackage, tmpDir string) (string, error) {
	// Testing override: if InstallerBootstrapMode registry key is set, use test-specific flow
	if mode := getInstallerBootstrapMode(); mode != "" {
		return extractInstallerFromAgentPackageTestMode(ctx, pkg, tmpDir, mode)
	}

	// Production flow: try OCI layer, fall back to MSI extraction for older packages
	installerBinPath := filepath.Join(tmpDir, "datadog-installer.exe")
	if err := pkg.ExtractLayers(ctx, oci.DatadogPackageInstallerLayerMediaType, installerBinPath); err != nil { // Returns nil if the layer doesn't exist
		return "", fmt.Errorf("failed to extract layers: %w", err)
	}
	if _, err := os.Stat(installerBinPath); err != nil {
		// Fallback to the old method if the file/layer doesn't exist.
		// Expected for Agent versions earlier than 7.79.
		return extractInstallerFromOldAgentPackage(ctx, pkg, tmpDir)
	}
	return installerBinPath, nil
}

// extractInstallerFromAgentPackageTestMode forces a specific extraction
// path when the InstallerBootstrapMode registry key is set.
// This is ONLY used for testing to force a specific bootstrap path.
func extractInstallerFromAgentPackageTestMode(ctx context.Context, pkg *oci.DownloadedPackage, tmpDir string, mode string) (string, error) {
	switch mode {
	case "OCI":
		// Force OCI path - fail if installer layer is missing
		installerBinPath := filepath.Join(tmpDir, "datadog-installer.exe")
		if err := pkg.ExtractLayers(ctx, oci.DatadogPackageInstallerLayerMediaType, installerBinPath); err != nil {
			return "", fmt.Errorf("failed to extract installer layer: %w", err)
		}
		if _, err := os.Stat(installerBinPath); err != nil {
			return "", fmt.Errorf("installer layer not found in OCI package (InstallerBootstrapMode=OCI): %w", err)
		}
		return installerBinPath, nil
	case "MSI":
		// Force MSI fallback path
		return extractInstallerFromOldAgentPackage(ctx, pkg, tmpDir)
	default:
		return "", fmt.Errorf("unknown InstallerBootstrapMode: %s (expected OCI or MSI)", mode)
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

// extractInstallerFromOldAgentPackage extracts datadog-installer.exe from
// the MSI carried in an already-downloaded Agent OCI package via MSI
// admin install.
//
// Should only be called for Agent versions earlier than 7.79 (where the
// dedicated installer.exe OCI layer is absent).
func extractInstallerFromOldAgentPackage(ctx context.Context, pkg *oci.DownloadedPackage, tmpDir string) (_ string, err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "bootstrap.download_installer_msi_fallback")
	defer func() { span.Finish(err) }()
	layoutTmpDir, err := os.MkdirTemp(paths.RootTmpDir, "layout")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(layoutTmpDir)
	if err := pkg.WriteOCILayout(ctx, layoutTmpDir); err != nil {
		return "", fmt.Errorf("failed to write OCI layout: %w", err)
	}

	if err := pkg.ExtractLayers(ctx, oci.DatadogPackageLayerMediaType, tmpDir); err != nil {
		return "", fmt.Errorf("failed to extract layers: %w", err)
	}

	installPath, err := getInstallerPath(ctx, tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to get installer path: %w", err)
	}

	return installPath, nil
}

// getInstallerPath returns the path to the installer binary inside the extraced OCI package.
//   - For package containing a MSI: extracts the installer from the MSI.
//   - else, falls back to finding the installer in the directory.
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

// getInstallerFromMSI extracts the installer from the MSI via a MSI admin install.
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

// getInstallerFromOCI returns the path to the installer binary inside the extraced OCI package.
// Used as a fallback in case we ever migrate away from shipping the MSI in the OCI package.
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
