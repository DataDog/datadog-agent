// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && !landlocktsync

package landlock

import (
	"fmt"

	ll "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"golang.org/x/sys/unix"
)

func enforceRulesetBeforeTSync(rulesetFD int) error {
	if err := ll.AllThreadsPrctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("all-threads prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}
	if err := ll.AllThreadsLandlockRestrictSelf(rulesetFD, 0); err != nil {
		return fmt.Errorf("all-threads landlock_restrict_self: %w", err)
	}
	return nil
}
