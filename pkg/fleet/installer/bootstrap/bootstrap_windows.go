// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	iexec "github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

func install(ctx context.Context, env *env.Env, url string, experiment bool) error {
	err := paths.EnsureInstallerDataDir()
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
		return err
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
		return nil, fmt.Errorf("failed to download installer package: %w", err)
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
		msi.WithAdditionalArgs([]string{fmt.Sprintf(`TARGETDIR="%s"`, strings.ReplaceAll(adminInstallDir, "/", `\`))}),
	)
	var output []byte
	if err == nil {
		output, err = cmd.Run(ctx)
	}

	if err != nil {
		return "", fmt.Errorf("failed to install the Datadog Installer: %w\n%s", err, string(output))
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
