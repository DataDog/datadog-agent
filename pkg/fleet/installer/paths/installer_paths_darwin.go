// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package paths defines commonly used paths throughout the installer
package paths

import "os"

// macOS has no FHS equivalent to hang the Agent off of: everything the Agent owns lives under
// the single install root /opt/datadog-agent, which holds the binaries alongside etc, etc-exp,
// run and logs. The Linux values these were split away from are all FHS paths that happen to
// exist on macOS, so leaving them in place would compile, run, and only fail on a real machine.
//
// These values must agree with pkg/util/defaultpaths/path_darwin.go, which the Agent itself
// resolves its configuration and run directories through.
const (
	// PackagesPath is the path to the packages directory.
	// Nothing this platform does today writes there -- macOS has no versioned package pool --
	// but the shared installer code names it unconditionally, so it keeps its Linux value.
	PackagesPath = "/opt/datadog-packages"
	// ConfigsPath is the path to the Fleet-managed configuration directory.
	ConfigsPath = "/opt/datadog-agent/etc/managed"
	// RootTmpDir is the temporary path where the bootstrapper will be extracted to.
	RootTmpDir = "/opt/datadog-packages/tmp"
	// DefaultUserConfigsDir is the default Agent configuration directory.
	DefaultUserConfigsDir = "/opt/datadog-agent"
	// AgentConfigDir is the path to the agent configuration directory.
	AgentConfigDir = "/opt/datadog-agent/etc"
	// AgentConfigDirExp is the path to the agent configuration directory for experiments.
	// It is a sibling of AgentConfigDir, not a child: the configuration copy, the recursive
	// ownership pass over etc and the first-install save-and-restore must all leave it alone.
	AgentConfigDirExp = "/opt/datadog-agent/etc-exp"
	// StableInstallerPath is the path to the stable installer binary.
	// macOS ships the installer inside the Agent package rather than as a package of its own,
	// so it sits in the install root and is the same binary for the life of the machine.
	StableInstallerPath = "/opt/datadog-agent/embedded/bin/installer"
	// ExperimentInstallerPath is the path to the experiment installer binary. macOS runs no
	// version experiments, so there is no second binary to point at.
	ExperimentInstallerPath = StableInstallerPath
	// RunPath is the default run path
	RunPath = "/opt/datadog-agent/run"
	// DatadogDataDir is the path to the Datadog data directory.
	DatadogDataDir = "/opt/datadog-agent/etc"
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
