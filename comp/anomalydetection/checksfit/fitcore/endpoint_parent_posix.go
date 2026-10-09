// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: this check moved out of setup.go so the transport still
// compiles on platforms without POSIX directory ownership metadata.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"syscall"
)

// endpointParentChecks requires an existing private parent directory owned
// by the effective user, and a path that fits sockaddr_un on both OSes.
func endpointParentChecks(path string) error {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return invalid("socket path must have a parent directory")
	}
	parent := path[:index]
	if parent == "" {
		parent = "/"
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(syscall.Geteuid()) || uint32(stat.Mode)&0o077 != 0 {
		return fmt.Errorf("socket parent must be a directory owned by this user with no group/other access: %w", fs.ErrPermission)
	}
	// sockaddr_un has a 104-byte path on macOS and 108 bytes on Linux,
	// including the NUL terminator.
	if len(path) >= 104 {
		return invalid("socket path is too long for macOS pathname Unix sockets")
	}
	return nil
}
