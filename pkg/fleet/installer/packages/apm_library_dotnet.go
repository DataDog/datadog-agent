// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
)

const (
	packageAPMLibraryDotnet = "datadog-apm-library-dotnet"
)

// SetupAPMLibraryDotnet runs on the first install of the .NET APM library after the files are laid out on disk.
func SetupAPMLibraryDotnet(_ context.Context, _ string) error {
	return nil
}

// StartAPMLibraryDotnetExperiment starts a .NET APM library experiment.
func StartAPMLibraryDotnetExperiment(_ context.Context) error {
	return nil
}

// StopAPMLibraryDotnetExperiment stops a .NET APM library experiment.
func StopAPMLibraryDotnetExperiment(_ context.Context) error {
	return nil
}

// PromoteAPMLibraryDotnetExperiment promotes a .NET APM library experiment.
func PromoteAPMLibraryDotnetExperiment(_ context.Context) error {
	return nil
}

// RemoveAPMLibraryDotnet removes the .NET APM library.
func RemoveAPMLibraryDotnet(_ context.Context) error {
	return nil
}

// PreRemoveHookDotnet runs before the garbage collector deletes the package files for a version.
func PreRemoveHookDotnet(_ context.Context, _ string) (bool, error) {
	return true, nil
}
