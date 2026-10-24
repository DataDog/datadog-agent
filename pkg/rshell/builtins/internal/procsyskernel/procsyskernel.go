// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procsyskernel reads Linux kernel information from /proc/sys/kernel/.
//
// This package is in builtins/internal/ and is therefore exempt from the
// builtinAllowedSymbols allowlist check. It may use OS-specific APIs freely.
//
// # Sandbox bypass
//
// ReadFile intentionally bypasses the AllowedPaths sandbox (callCtx.OpenFile)
// and calls os.OpenFile directly. This is safe because procPath is always a
// kernel-managed pseudo-filesystem root (/proc by default) that is hardcoded
// by the caller — it is never derived from user-supplied input and cannot be
// redirected by a shell script. The caller is responsible for ensuring that
// procPath remains a safe, non-user-controlled path.
package procsyskernel

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ReadFile reads a single-line value from a /proc/sys/kernel/ pseudo-file.
// name is the filename (e.g. "ostype", "hostname"). procPath is the base
// proc path (e.g. "/proc" or "/host/proc").
//
// The file is opened with O_NONBLOCK to prevent blocking on FIFOs, then
// validated via fstat to reject non-regular files. Reads are bounded to
// 4 KiB. The returned value is trimmed of trailing whitespace.
func ReadFile(procPath, name string) (string, error) {
	// Defence-in-depth: reject ".." in the original path before Clean
	// so traversal like "/proc/../etc/passwd" is caught. Matches the
	// equivalent guard in procnetroute and procnetsocket.
	if strings.Contains(procPath, "..") {
		return "", fmt.Errorf("procsyskernel: unsafe procPath %q (must not contain \"..\" components)", procPath)
	}
	// Reject path components in name — must be a plain basename (e.g. "ostype").
	if name != filepath.Base(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("procsyskernel: unsafe name %q (must be a plain filename)", name)
	}
	path := filepath.Join(filepath.Clean(procPath), "sys", "kernel", name)
	// Open with O_NONBLOCK to prevent blocking on FIFOs, then validate
	// the file type via fstat on the opened fd. This is atomic — no
	// TOCTOU gap between type check and open.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() && info.Mode().Type()&os.ModeCharDevice == 0 {
		// Allow regular files and char devices (proc pseudo-files appear as
		// char devices on some configurations). Reject FIFOs, sockets, etc.
		return "", fmt.Errorf("not a regular file: %s", path)
	}
	// Proc kernel files are tiny single-line values. Cap at 4 KiB.
	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), " \t\r\n"), nil
}
