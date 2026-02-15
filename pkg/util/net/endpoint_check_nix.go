// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package net

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// IsUDSAvailable checks if the given path refers to an existing Unix domain socket
// that the current process can write to. This is intended for verifying that a
// DogStatsD UDS datagram socket is ready to receive data.
//
// It checks three things:
//  1. The path exists on disk.
//  2. The file is a socket.
//  3. The current process has write permission.
func IsUDSAvailable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return false
	}
	if err := syscall.Access(path, unix.W_OK); err != nil {
		return false
	}
	return true
}
