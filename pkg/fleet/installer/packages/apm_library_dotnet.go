// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
)

// SetupAPMLibraryDotnet installs the .NET APM library.
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
