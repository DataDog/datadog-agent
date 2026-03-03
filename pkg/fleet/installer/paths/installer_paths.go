// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package paths defines commonly used paths throughout the installer
package paths

import "os"

const (
	// PackagesPath is the path to the packages directory.
	PackagesPath = "/opt/datadog-packages"
	// ConfigsPath is the path to the Fleet-managed configuration directory.
	ConfigsPath = "/etc/datadog-agent/managed"
	// RootTmpDir is the temporary path where the bootstrapper will be extracted to.
	RootTmpDir = "/opt/datadog-packages/tmp"
	// DefaultUserConfigsDir is the default Agent configuration directory.
	DefaultUserConfigsDir = "/etc"
	// AgentConfigDir is the path to the agent configuration directory.
	AgentConfigDir = "/etc/datadog-agent"
	// AgentConfigDirExp is the path to the agent configuration directory for experiments.
	AgentConfigDirExp = "/etc/datadog-agent-exp"
	// StableInstallerPath is the path to the stable installer binary.
	StableInstallerPath = "/opt/datadog-packages/datadog-installer/stable/bin/installer/installer"
	// ExperimentInstallerPath is the path to the experiment installer binary.
	ExperimentInstallerPath = "/opt/datadog-packages/datadog-installer/experiment/bin/installer/installer"
	// RunPath is the default run path
	RunPath = "/opt/datadog-packages/run"
	// DatadogDataDir is the path to the Datadog data directory.
	DatadogDataDir = "/etc/datadog-agent"
	// DatadogProgramFilesDir is the Datadog Program Files directory (not used on non-Windows platforms).
	DatadogProgramFilesDir = ""
)

// SetupInstallerDataDir ensures that permissions are set correctly on the installer data directory.
// This is a no-op on non-Windows platforms.
func SetupInstallerDataDir() error {
	return nil
}

// EnsureInstallerDataDir ensures that permissions are set correctly on the installer data directory.
// This is a no-op on non-Windows platforms.
func EnsureInstallerDataDir() error {
	return nil
}

// SetRepositoryPermissions sets the permissions on the repository directory
func SetRepositoryPermissions(path string) error {
	return os.Chmod(path, 0755)
}
