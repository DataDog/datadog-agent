// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procnetsocket reads Linux socket state from /proc/net/.
//
// This package is in builtins/internal/ and is therefore exempt from the
// builtinAllowedSymbols allowlist check. It may use OS-specific APIs freely.
//
// # Sandbox bypass
//
// All Read* functions intentionally bypass the AllowedPaths sandbox
// (callCtx.OpenFile) and call os.Open directly. This is safe because procPath
// is always a kernel-managed pseudo-filesystem root (/proc by default) that is
// hardcoded by the caller — it is never derived from user-supplied input and
// cannot be redirected by a shell script. The caller is responsible for
// ensuring that procPath remains a safe, non-user-controlled path.
package procnetsocket

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath"
)

// DefaultProcPath is the default proc filesystem root.
const DefaultProcPath = procpath.Default

// MaxLineBytes is the per-line buffer cap for the /proc/net/ scanner.
const MaxLineBytes = 1 << 20 // 1 MiB

// MaxEntries caps the number of socket entries retained in memory per
// /proc/net/ file to prevent memory exhaustion on hosts with very large
// socket tables.
const MaxEntries = 100_000

// MaxTotalLines caps the total number of lines (valid + malformed/skipped)
// scanned per Read* call. This bounds CPU time for pathological files with
// many malformed/non-matching lines before MaxEntries valid entries are found.
// MaxEntries is the memory guard; MaxTotalLines is the scan-time guard.
const MaxTotalLines = MaxEntries * 10 // 1 000 000 lines

// ErrMaxEntries is returned when the socket table exceeds MaxEntries entries.
// Callers should treat this as a hard failure: the table was truncated and
// output may be missing active sockets.
var ErrMaxEntries = errors.New("procnetsocket: socket table truncated: exceeded MaxEntries limit")

// ErrMaxTotalLines is returned when more than MaxTotalLines lines are scanned.
// Callers should treat this as a hard failure: the table was truncated and
// output may be missing active sockets.
var ErrMaxTotalLines = errors.New("procnetsocket: socket table truncated: exceeded MaxTotalLines limit")

// SocketKind identifies the protocol family of a parsed socket entry.
type SocketKind int

const (
	KindTCP4 SocketKind = iota
	KindTCP6
	KindUDP4
	KindUDP6
	KindUnix
)

// SocketEntry holds a parsed socket entry from /proc/net/.
type SocketEntry struct {
	Kind        SocketKind
	State       string
	RecvQ       uint64
	SendQ       uint64
	LocalAddr   string
	LocalPort   string
	PeerAddr    string
	PeerPort    string
	UID         uint32
	Inode       uint64
	HasExtended bool
}

// validateProcPath rejects any procPath that contains ".." components and
// returns the cleaned path for use in subsequent file operations.
// The check is applied to the ORIGINAL path (before filepath.Clean) so that
// traversal sequences like "/proc/../etc/passwd" are caught — after Clean,
// such a path becomes "/etc/passwd" which no longer contains "..".
// Defence-in-depth: procPath is always a hardcoded kernel pseudo-filesystem
// root in production and never derived from user input, so this check should
// never trigger. It mirrors the equivalent guard in procnetroute.ReadRoutes
// and ensures the invariant is enforced consistently across both packages.
func validateProcPath(procPath string) (string, error) {
	if strings.Contains(procPath, "..") {
		return "", fmt.Errorf("procnetsocket: unsafe procPath %q (must not contain \"..\" components)", procPath)
	}
	return filepath.Clean(procPath), nil
}

// ReadTCP4 reads procPath/net/tcp and returns IPv4 TCP socket entries.
//
// Sandbox bypass: os.Open is used directly; path is derived from procPath, a
// hardcoded kernel pseudo-filesystem root never supplied by user input.
//
// Defence-in-depth: ".." components are always rejected regardless of context.
func ReadTCP4(ctx context.Context, procPath string) ([]SocketEntry, error) {
	clean, err := validateProcPath(procPath)
	if err != nil {
		return nil, err
	}
	return readTCP4(ctx, clean)
}

// ReadTCP6 reads procPath/net/tcp6 and returns IPv6 TCP socket entries.
//
// Sandbox bypass: same rationale as ReadTCP4.
// Defence-in-depth: same ".." guard as ReadTCP4.
func ReadTCP6(ctx context.Context, procPath string) ([]SocketEntry, error) {
	clean, err := validateProcPath(procPath)
	if err != nil {
		return nil, err
	}
	return readTCP6(ctx, clean)
}

// ReadUDP4 reads procPath/net/udp and returns IPv4 UDP socket entries.
//
// Sandbox bypass: same rationale as ReadTCP4.
// Defence-in-depth: same ".." guard as ReadTCP4.
func ReadUDP4(ctx context.Context, procPath string) ([]SocketEntry, error) {
	clean, err := validateProcPath(procPath)
	if err != nil {
		return nil, err
	}
	return readUDP4(ctx, clean)
}

// ReadUDP6 reads procPath/net/udp6 and returns IPv6 UDP socket entries.
//
// Sandbox bypass: same rationale as ReadTCP4.
// Defence-in-depth: same ".." guard as ReadTCP4.
func ReadUDP6(ctx context.Context, procPath string) ([]SocketEntry, error) {
	clean, err := validateProcPath(procPath)
	if err != nil {
		return nil, err
	}
	return readUDP6(ctx, clean)
}

// ReadUnix reads procPath/net/unix and returns Unix domain socket entries.
//
// Sandbox bypass: same rationale as ReadTCP4.
// Defence-in-depth: same ".." guard as ReadTCP4.
func ReadUnix(ctx context.Context, procPath string) ([]SocketEntry, error) {
	clean, err := validateProcPath(procPath)
	if err != nil {
		return nil, err
	}
	return readUnix(ctx, clean)
}
