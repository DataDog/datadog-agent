// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package privilegedhelper

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// minLandlockABI is the Landlock ABI the helper sandbox requires.
const minLandlockABI = 3

var (
	landlockABIVersion = func() (int, error) {
		// Querying the ABI version needs no privileges.
		version, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
		if errno != 0 {
			return 0, errno
		}
		return int(version), nil
	}
	execve              = syscall.Exec
	ensureTrustedHelper = ensureTrustedHelperBinary
)

// startHelper replaces this process with the rshell privileged helper, which
// creates and serves its socket. systemd keeps supervising it as the
// service's main process.
func startHelper(logger log.Component, params *helperParams) error {
	if params.helperPath == "" {
		return errors.New("--helper is required")
	}
	if version, err := landlockABIVersion(); err != nil || version < minLandlockABI {
		logger.Warnf("not starting the rshell privileged helper: it needs Landlock ABI %d (got %d, %v)", minLandlockABI, version, err)
		return nil
	}
	// This launcher runs as root, so refuse to exec a helper binary that
	// dd-agent could have tampered with: a symlink (whose target could be
	// swapped), a non-root-owned file, or one writable by group or other.
	if err := ensureTrustedHelper(params.helperPath); err != nil {
		return err
	}
	logger.Infof("starting the rshell privileged helper on %s", params.socketPath)
	// exec discards whatever the logger has not written yet.
	logger.Flush()
	args := []string{params.helperPath, "privileged-helper",
		"--socket=" + params.socketPath, "--policy=" + params.policyPath, "--idle-timeout=0"}
	// The helper runs commands with an empty environment; pass it nothing
	// from the Agent's, which can hold credentials.
	err := execve(params.helperPath, args, []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"})
	return fmt.Errorf("exec the rshell privileged helper: %w", err)
}

// ensureTrustedHelperBinary refuses to exec a helper binary that a non-root
// user could control. It must be a regular file (not a symlink whose target
// could be swapped), owned by root, and not writable by group or other.
func ensureTrustedHelperBinary(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat the rshell privileged helper %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to run the rshell privileged helper: %q is not a regular file", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("refusing to run the rshell privileged helper: cannot read ownership of %q", path)
	}
	if stat.Uid != 0 {
		return fmt.Errorf("refusing to run the rshell privileged helper: %q is owned by uid %d, not root", path, stat.Uid)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("refusing to run the rshell privileged helper: %q is writable by group or other (mode %#o)", path, info.Mode().Perm())
	}
	return nil
}
