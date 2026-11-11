// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && !darwin

package allowedpaths

import (
	"errors"

	"golang.org/x/sys/unix"
)

func fifoFDReadReady(fd uintptr) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if _, err := unix.Poll(fds, 0); err != nil {
		return false, err
	}
	if fds[0].Revents&unix.POLLNVAL != 0 {
		return false, errors.New("invalid FIFO descriptor")
	}
	return fds[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0, nil
}
