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
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	iexec "github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

func install(ctx context.Context, env *env.Env, url string, experiment bool) error {
	err := paths.CreateInstallerDataDir()
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
		return fmt.Errorf("failed to download installer: %w", err)
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
	if downloadedPackage.Name != InstallerPackage {
		return nil, fmt.Errorf("unexpected package name: %s, expected %s", downloadedPackage.Name, InstallerPackage)
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

	msis, err := filepath.Glob(filepath.Join(tmpDir, "datadog-installer-*-1-x86_64.msi"))
	if err != nil {
		return nil, err
	}
	if len(msis) > 1 {
		return nil, fmt.Errorf("too many MSIs in package")
	} else if len(msis) == 0 {
		return nil, fmt.Errorf("no MSIs in package")
	}

	adminInstallDir := path.Join(tmpDir, "datadog-installer")

	cmd, err := msi.Cmd(
		msi.AdministrativeInstall(),
		msi.WithMsi(msis[0]),
		msi.WithAdditionalArgs([]string{fmt.Sprintf(`TARGETDIR="%s"`, strings.ReplaceAll(adminInstallDir, "/", `\`))}),
	)
	var output []byte
	if err == nil {
		output, err = cmd.Run()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to install the Datadog Installer: %w\n%s", err, string(output))
	}
	return iexec.NewInstallerExec(env, path.Join(adminInstallDir, "ProgramFiles64Folder", "Datadog", "Datadog Installer", "datadog-installer.exe")), nil
}
