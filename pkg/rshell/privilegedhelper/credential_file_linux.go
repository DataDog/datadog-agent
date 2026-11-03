// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package privilegedhelper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func readCredentialFile(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open verification credential: %w", err)
	}
	file := os.NewFile(uintptr(fd), "rshell verification credential")
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("open verification credential: invalid file descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat verification credential: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("verification credential must be a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return nil, errors.New("verification credential must be owned by root")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("verification credential must not be group- or world-writable")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxMessageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read verification credential: %w", err)
	}
	if len(data) > MaxMessageBytes {
		return nil, fmt.Errorf("verification credential exceeds %d bytes", MaxMessageBytes)
	}
	return data, nil
}
