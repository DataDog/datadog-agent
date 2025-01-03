// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements the installer downloader
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/installer/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
)

const (
	installerPackage = "datadog-installer"
	installerBinPath = "bin/installer/installer"
)

var (
	// Version is the version of the installer to download.
	Version string
	// Flavor is the flavor of the setup to run.
	Flavor string
)

func main() {
	if Version == "" || Flavor == "" {
		fmt.Fprintln(os.Stderr, "Version and Flavor must be set at build time.")
		os.Exit(1)
	}
	if !user.IsRoot() {
		fmt.Fprintln(os.Stderr, "This installer requires root privileges.")
		os.Exit(1)
	}
	env := env.FromEnv()
	ctx := context.Background()

	t := telemetry.NewTelemetry(env.HTTPClient(), env.APIKey, env.Site, fmt.Sprintf("datadog-installer-downloader-%s", Flavor))
	var err error
	span, ctx := telemetry.StartSpanFromEnv(ctx, fmt.Sprintf("downloader-%s", Flavor))
	err = runDownloader(ctx, env, Version, Flavor)

	span.Finish(err)
	t.Stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		os.Exit(1)
	}
}

func runDownloader(ctx context.Context, env *env.Env, version string, flavor string) error {
	downloaderPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	tmpDir, err := os.MkdirTemp(filepath.Dir(downloaderPath), "datadog-installer")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = downloadInstaller(ctx, env, version, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to download installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(tmpDir, installerBinPath), "setup", "--flavor", flavor)
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), telemetry.EnvFromContext(ctx)...)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run installer: %w", err)
	}
	return nil
}

func downloadInstaller(ctx context.Context, env *env.Env, version string, tmpDir string) error {
	url := oci.PackageURL(env, installerPackage, version)
	downloader := oci.NewDownloader(env, env.HTTPClient())
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to download installer package: %w", err)
	}
	if downloadedPackage.Name != installerPackage {
		return fmt.Errorf("unexpected package name: %s, expected %s", downloadedPackage.Name, installerPackage)
	}
	err = downloadedPackage.WriteOCILayout(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to write OCI layout: %w", err)
	}
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to extract layers: %w", err)
	}
	return nil
}
