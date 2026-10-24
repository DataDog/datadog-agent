// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && !darwin

package awk

import "golang.org/x/sys/unix"

func programFileReadReady(fd uintptr) (bool, error) {
	ready, err := unix.Poll([]unix.PollFd{{
		Fd:     int32(fd),
		Events: unix.POLLIN | unix.POLLHUP,
	}}, programReadWaitMilliseconds)
	return ready > 0, err
}
