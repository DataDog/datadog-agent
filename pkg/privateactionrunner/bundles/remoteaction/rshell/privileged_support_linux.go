// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package com_datadoghq_remoteaction_rshell

import (
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

// privilegedHelperMinLandlockABI is the Landlock ABI the privileged helper
// sandbox requires.
const privilegedHelperMinLandlockABI = 3

var landlockABIVersion = sync.OnceValues(func() (int, error) {
	// Querying the ABI version needs no privileges.
	version, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0, errno
	}
	return int(version), nil
})

// checkPrivilegedHelperSupported reports why this host cannot run the
// privileged helper, or nil when it can. The helper would accept the
// connection and then fail, which is not safe to retry locally, so the check
// happens before dialing.
var checkPrivilegedHelperSupported = func() error {
	version, err := landlockABIVersion()
	if err != nil {
		return fmt.Errorf("landlock is unavailable: %w", err)
	}
	if version < privilegedHelperMinLandlockABI {
		return fmt.Errorf("landlock ABI %d is older than the required %d", version, privilegedHelperMinLandlockABI)
	}
	return nil
}
