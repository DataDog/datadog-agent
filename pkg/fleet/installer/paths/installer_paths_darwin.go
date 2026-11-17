// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package paths defines commonly used paths throughout the installer
package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// macOS has no FHS equivalent to hang the Agent off of: everything the Agent owns lives under
// the single install root /opt/datadog-agent, which holds the binaries alongside etc, etc-exp,
// run and logs. The Linux values these were split away from are all FHS paths that happen to
// exist on macOS, so leaving them in place would compile, run, and only fail on a real machine.
//
// These values must agree with pkg/util/defaultpaths/path_darwin.go, which the Agent itself
// resolves its configuration and run directories through.
const (
	// PackagesPath is the path to the packages directory.
	// macOS has no versioned package pool of its own: the Agent comes from a .dmg and lives in
	// the install root. The shared installer code reads a package repository per package all the
	// same, so the install registers a placeholder one here (see registerPackageRepository in
	// pkg/fleet/installer/packages/datadog_agent_darwin.go) and the path keeps its Linux value.
	PackagesPath = "/opt/datadog-packages"
	// ConfigsPath is empty: macOS never had the legacy managed/ policy layout, so there is no
	// directory for the installer to create or purge.
	ConfigsPath = ""
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
	// RunPath is the root installer daemon's own run directory: its socket, pid file, task
	// database, remote configuration cache and the configuration experiment deadline.
	//
	// It is deliberately not the Agent's run directory, /opt/datadog-agent/run: that one belongs
	// to the Agent's account, and the daemon runs as root. Like on Linux, it lives under
	// PackagesPath, which only root can write to (see EnsureInstallerDataDir).
	RunPath = "/opt/datadog-packages/run"
	// InstallerLogPath is where the root installer daemon's logs go: its launchd job's standard
	// output and error, and its own log file. Root-owned for the same reason as RunPath: the
	// Agent's log directory belongs to the Agent's account, and a root process writing in it
	// would follow any link that account planted there.
	InstallerLogPath = "/opt/datadog-packages/logs"
	// DatadogDataDir is the path to the Datadog data directory.
	DatadogDataDir = "/opt/datadog-agent/etc"
	// DatadogProgramFilesDir is the Datadog Program Files directory (not used on non-Windows platforms).
	DatadogProgramFilesDir = ""
)

// ConfigDirIsTrustedForRead permits reads on non-Windows platforms, where this Windows
// directory-ownership policy does not apply.
func ConfigDirIsTrustedForRead(_ string) (bool, error) {
	return true, nil
}

// SetupInstallerDataDir ensures that permissions are set correctly on the installer data directory.
func SetupInstallerDataDir() error {
	return EnsureInstallerDataDir()
}

// rootOwner is the owner every directory holding root installer state must have: root:wheel.
var rootOwner = dirOwner{uid: 0, gid: 0}

// EnsureInstallerDataDir makes sure the directories the root installer keeps its own state in,
// PackagesPath, RunPath and InstallerLogPath, are real directories owned by root:wheel and writable by root only,
// creating them when missing and repairing their ownership and mode when they drifted.
//
// The root daemon, its detached watcher and its resume step trust what these directories hold,
// so they must never be writable by the Agent's account. A path that is not a directory (a
// symbolic link in particular) is refused rather than repaired.
func EnsureInstallerDataDir() error {
	for _, dir := range []string{PackagesPath, RunPath, InstallerLogPath} {
		if err := ensureTrustedDir(dir, rootOwner); err != nil {
			return err
		}
	}
	return nil
}

// dirOwner is the uid and gid ensureTrustedDir enforces. A gid of -1 leaves the group alone.
type dirOwner struct {
	uid int
	gid int
}

// ensureTrustedDir creates dir with mode 0755 if it is missing, refuses it if it is not a real
// directory, and repairs its owner and mode if it belongs to someone else or is writable by its
// group or by others. Its parent must already be trusted.
func ensureTrustedDir(dir string, owner dirOwner) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(dir, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("could not create %s: %w", dir, err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to use %s: not a directory", dir)
	}
	if trusted(info, owner) {
		return nil
	}
	// Lchown and Chmod act on the directory Lstat just saw: its parent is trusted, so nobody else
	// can have swapped it for a link in the meantime.
	if err := os.Lchown(dir, owner.uid, owner.gid); err != nil {
		return fmt.Errorf("could not repair the ownership of %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("could not repair the mode of %s: %w", dir, err)
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", dir, err)
	}
	if !info.IsDir() || !trusted(info, owner) {
		return fmt.Errorf("refusing to use %s: could not make it owned and writable by uid %d only", dir, owner.uid)
	}
	return nil
}

// trusted reports whether info describes a directory owned by owner and writable by no one else.
func trusted(info os.FileInfo, owner dirOwner) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if int(st.Uid) != owner.uid || (owner.gid >= 0 && int(st.Gid) != owner.gid) {
		return false
	}
	return info.Mode().Perm()&0o022 == 0
}

// SetRepositoryPermissions sets the permissions on the repository directory
func SetRepositoryPermissions(path string) error {
	return os.Chmod(path, 0755)
}
