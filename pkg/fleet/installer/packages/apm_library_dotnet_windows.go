// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	packageAPMLibraryDotnet = "datadog-apm-library-dotnet"
)

var (
	installerRelativePath = []string{"installer", "Datadog.FleetInstaller.exe"}
)

func getTargetPath(target string) string {
	return filepath.Join(paths.PackagesPath, packageAPMLibraryDotnet, target)
}

func getExecutablePath(installDir string) string {
	return filepath.Join(append([]string{installDir}, installerRelativePath...)...)
}

func getLibraryPath(installDir string) string {
	return filepath.Join(installDir, "library")
}

// SetupAPMLibraryDotnet installs the .NET APM library.
func SetupAPMLibraryDotnet(ctx context.Context, _ string) (err error) {
	// Register GAC + set env variables
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	err = dotnetExec.Install(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return nil
}

// StartAPMLibraryDotnetExperiment starts a .NET APM library experiment.
func StartAPMLibraryDotnetExperiment(ctx context.Context) (err error) {
	// Register GAC + set env variables new version
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("experiment"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	err = dotnetExec.Install(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return nil
}

// StopAPMLibraryDotnetExperiment stops a .NET APM library experiment.
func StopAPMLibraryDotnetExperiment(ctx context.Context) (err error) {
	// Re-register GAC + set env variables of stable version
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	err = dotnetExec.Install(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return nil
}

// PromoteAPMLibraryDotnetExperiment promotes a .NET APM library experiment to stable.
func PromoteAPMLibraryDotnetExperiment(_ context.Context) (err error) {
	// Do nothing since the experiment is already installed and does not rely on the symlink
	return nil
}

// RemoveAPMLibraryDotnet uninstalls the .NET APM library.
func RemoveAPMLibraryDotnet(ctx context.Context) (err error) {
	// Unregister GAC + unset env variables
	var installDir string
	installDir, err = filepath.EvalSymlinks(getTargetPath("stable"))
	if err != nil {
		return err
	}
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(installDir))
	err = dotnetExec.UninstallProduct(ctx)
	if err != nil {
		return err
	}
	err = dotnetExec.UninstallVersion(ctx, getLibraryPath(installDir))
	if err != nil {
		return err
	}
	return nil
}

// GarbageCollectAPMLibraryDotnet runs before the garbage collector deletes the package files for a version.
func GarbageCollectAPMLibraryDotnet(pkgPath string) (err error) {
	ctx := context.Background()
	dotnetExec := exec.NewDotnetLibraryExec(getExecutablePath(pkgPath))
	err = dotnetExec.UninstallVersion(ctx, getLibraryPath(pkgPath))
	if err != nil {
		return err
	}
	return nil
}
