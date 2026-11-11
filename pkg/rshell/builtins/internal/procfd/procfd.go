// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procfd provides Linux open-file-descriptor enumeration for the
// lsof builtin.
//
// Only Linux is supported; macOS and Windows return ErrNotSupported (the
// same shape as builtins/internal/meminfo, used by the Linux-only free
// builtin).
//
// This package is in builtins/internal/ and is therefore exempt from the
// per-command builtinAllowedSymbols check applied to files under builtins/.
// Its own stdlib/unix symbol usage is still tracked in
// analysis/symbols_internal.go's internalAllowedSymbols and
// internalPerPackageSymbols["procfd"].
//
// Privacy: this package never reads a process's argv/cmdline or environ.
// The Command field is the comm name only (matching builtins/internal/
// procinfo), not the full argv[0] or any argument.
package procfd

import (
	"context"
	"errors"
)

// MaxProcesses caps the number of processes scanned, matching
// procinfo.MaxProcesses.
const MaxProcesses = 10_000

// MaxFDsPerProcess caps the number of file descriptors read per process to
// bound work against processes with pathologically large (or spoofed)
// open-file counts.
const MaxFDsPerProcess = 10_000

// MaxTotalOpenFiles caps the aggregate number of OpenFile entries returned
// by a single List call, across every scanned process. MaxProcesses and
// MaxFDsPerProcess each bound one dimension, but their product (up to 100M
// entries) is still an unbounded-memory risk for a plain, selector-less
// scan; this caps the sum directly.
const MaxTotalOpenFiles = 100_000

// OpenFile describes a single open file descriptor (or descriptor-like
// entry: cwd, root, exe) held by a process.
type OpenFile struct {
	PID     int
	Command string // comm name only; never argv
	UID     string // real UID, as a numeric string
	FD      string // "cwd", "rtd", "txt", or a numeric descriptor
	Type    string // REG, DIR, CHR, BLK, FIFO, sock, a_inode, unknown
	Device  string // "major,minor" of the underlying device, "" if unknown
	Size    string // file size in bytes as a decimal string, "" if unknown
	Node    string // inode number as a decimal string, "" if unknown
	Name    string // resolved target; a filesystem path when IsPath is true

	// IsPath reports whether Name is a filesystem path that must be
	// checked against the caller's AllowedPaths configuration before
	// being shown. Non-path targets (sockets, pipes, anonymous inodes)
	// are never gated.
	IsPath bool

	// Deleted reports whether the kernel marked this descriptor's target
	// as unlinked (the " (deleted)" suffix on the /proc/<pid>/fd symlink
	// target). Name has the suffix stripped; callers re-append a marker
	// as needed.
	Deleted bool
}

// ErrNotSupported is returned by List on platforms without a backend.
var ErrNotSupported = errors.New("not supported on this platform")

// ProcessFilter reports whether a process, identified by its PID, comm name,
// and real UID, should be scanned for open files. It is evaluated once per
// process, before that process's fd directory is scanned, so a non-matching
// process's file descriptors never consume any of the caller's
// MaxTotalOpenFiles budget. A nil filter matches every process.
type ProcessFilter func(pid int, comm, uid string) bool

// List returns open file descriptors for the given PIDs. A nil or empty
// pids selects every process visible under procPath (bounded by
// MaxProcesses). Missing or inaccessible PIDs are silently skipped, since
// processes can exit between listing and reading. filter, if non-nil,
// restricts scanning to processes it accepts.
func List(ctx context.Context, procPath string, pids []int, filter ProcessFilter) ([]OpenFile, error) {
	return list(ctx, procPath, pids, filter)
}
