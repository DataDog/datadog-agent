// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package apmlibrarydotnet contains shared helpers for the .NET APM library package.
package apmlibrarydotnet

import (
	"context"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/exec"
)

const (
	// PackageName is the package name for the .NET APM library.
	PackageName = "datadog-apm-library-dotnet"
)

var installerRelativePath = []string{"installer", "Datadog.FleetInstaller.exe"}

// ExecutablePath returns the path to the .NET library helper executable.
func ExecutablePath(installDir string) string {
	return filepath.Join(append([]string{installDir}, installerRelativePath...)...)
}

// LibraryPath returns the path to the .NET library home directory.
func LibraryPath(installDir string) string {
	return filepath.Join(installDir, "library")
}

// AsyncPreRemoveHook runs before the garbage collector deletes package files for a version.
func AsyncPreRemoveHook(ctx context.Context, pkgRepositoryPath string) (bool, error) {
	dotnetExec := exec.NewDotnetLibraryExec(ExecutablePath(pkgRepositoryPath))
	exitCode, err := dotnetExec.UninstallVersion(ctx, LibraryPath(pkgRepositoryPath))
	if err != nil {
		// We only block deletion if we could not delete the native loader files
		// cf https://github.com/DataDog/dd-trace-dotnet/blob/master/tracer/src/Datadog.FleetInstaller/ReturnCode.cs#L14
		const errorRemovingNativeLoaderFiles = 2
		shouldDelete := exitCode != errorRemovingNativeLoaderFiles
		return shouldDelete, err
	}
	return true, nil
}
