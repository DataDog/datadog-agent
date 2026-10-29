// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package awk

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Darwin reports a drained FIFO writer's disconnect through select, but not poll.
func programFileReadReady(fd uintptr) (bool, error) {
	if fd >= uintptr(unix.FD_SETSIZE) {
		return false, fmt.Errorf("program descriptor %d exceeds select limit", fd)
	}
	var readSet unix.FdSet
	readSet.Set(int(fd))
	timeout := unix.Timeval{Usec: programReadWaitMilliseconds * 1000}
	n, err := unix.Select(int(fd)+1, &readSet, nil, nil, &timeout)
	return n > 0 && readSet.IsSet(int(fd)), err
}
