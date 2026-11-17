// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package allowedpaths

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Darwin preserves an empty writer's disconnect as select readability, but
// does not expose it through poll or kqueue.
func fifoFDReadReady(fd uintptr) (bool, error) {
	if fd >= uintptr(unix.FD_SETSIZE) {
		return false, fmt.Errorf("FIFO descriptor %d exceeds select limit", fd)
	}
	var readSet unix.FdSet
	readSet.Set(int(fd))
	timeout := unix.Timeval{}
	n, err := unix.Select(int(fd)+1, &readSet, nil, nil, &timeout)
	return n > 0 && readSet.IsSet(int(fd)), err
}
