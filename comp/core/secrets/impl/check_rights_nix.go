// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package secretsimpl

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func checkRights(path string, allowGroupExec bool) error {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return fmt.Errorf("invalid executable '%s': can't stat it: %s", path, err)
	}

	if allowGroupExec {
		if stat.Mode&(syscall.S_IWGRP|syscall.S_IRWXO) != 0 {
			return fmt.Errorf("invalid executable '%s', 'others' have rights on it or 'group' has write permissions on it", path)
		}
	} else {
		if stat.Mode&(syscall.S_IRWXG|syscall.S_IRWXO) != 0 {
			return fmt.Errorf("invalid executable '%s', 'group' or 'others' have rights on it", path)
		}
	}

	if err := syscall.Access(path, unix.X_OK); err != nil {
		return fmt.Errorf("invalid executable '%s': can't access it: %s", path, err)
	}

	return nil
}
