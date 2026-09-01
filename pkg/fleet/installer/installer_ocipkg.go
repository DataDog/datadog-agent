// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

// installExperiment installs an OCI package as the experiment.
//
// This is the flow every platform but macOS takes, moved here unchanged from InstallExperiment so
// that the macOS flow can be a peer of it rather than a branch inside it. The caller holds the
// installer's lock.
func (i *installerImpl) installExperiment(ctx context.Context, url string) error {
	pkg, err := i.downloader.Download(ctx, url)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	err = checkAvailableDiskSpace(i.packages, pkg)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrNotEnoughDiskSpace,
			fmt.Errorf("not enough disk space: %w", err),
		)
	}
	tmpDir, err := i.packages.MkdirTemp()
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could create temporary directory: %w", err),
		)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.userConfigsDir, "datadog-agent")
	err = pkg.ExtractLayers(ctx, oci.DatadogPackageLayerMediaType, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not extract package layer: %w", err),
		)
	}
	err = pkg.ExtractLayers(ctx, oci.DatadogPackageConfigLayerMediaType, configDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not extract package config layer: %w", err),
		)
	}

	err = i.hooks.PreStartExperiment(ctx, pkg.Name)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	repository := i.packages.Get(pkg.Name)
	err = repository.SetExperiment(ctx, pkg.Version, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("could not set experiment: %w", err),
		)
	}
	err = i.config.RemoveExperiment(ctx)
	if err != nil {
		return fmt.Errorf("could not remove config experiment: %w", err)
	}
	// HACK: close so package can be updated as watchdog runs
	if pkg.Name == packageDatadogAgent && runtime.GOOS == "windows" {
		i.db.Close()
	}
	err = i.hooks.PostStartExperiment(ctx, pkg.Name)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	return nil
}
