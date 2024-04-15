// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

// SetupInstallerUnit noop
func SetupInstallerUnit() (err error) {
	return nil
}

// RemoveInstallerUnit noop
func RemoveInstallerUnit() {
}

// StartInstallerExperiment noop
func StartInstallerExperiment() error {
	return nil
}

// StopInstallerExperiment noop
func StopInstallerExperiment() error {
	return nil
}
