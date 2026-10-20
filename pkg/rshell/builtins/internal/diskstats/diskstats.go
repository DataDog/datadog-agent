// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package diskstats reads mounted-filesystem usage information from the
// kernel and presents it as a normalised cross-platform Mount struct.
//
// This package lives under builtins/internal/ and is therefore exempt from
// the builtinAllowedSymbols allowlist check. It may use OS-specific APIs
// freely.
//
// # Sandbox bypass
//
// The Linux backend reads /proc/self/mountinfo via os.Open directly,
// intentionally bypassing the AllowedPaths sandbox (callCtx.OpenFile). The
// path is a kernel-managed pseudo-file that is hardcoded by this package
// and never derived from user-supplied input, so AllowedPaths restrictions
// do not apply. This matches the documented exception used by the ss and
// ip route builtins.
//
// # Memory and CPU bounds
//
// MaxMounts caps the number of mount entries retained in memory. On hosts
// with very large mount tables (containers with thousands of bind mounts,
// for instance), the listing is truncated and ErrMaxMounts is returned.
//
// MaxMountInfoLine caps the per-line buffer size when scanning
// /proc/self/mountinfo. Lines longer than this cause the scan to fail with
// ErrLineTooLong.
package diskstats

import (
	"context"
	"errors"
)

// MaxMounts caps the number of mount entries kept in memory per List call.
// Mirrors procnetsocket.MaxEntries (100k). Exported so callers can quote
// it in user-facing truncation warnings.
const MaxMounts = 100_000

// maxMountInfoLine caps the per-line buffer size when scanning
// /proc/self/mountinfo. Lines longer than this cause the scan to fail.
const maxMountInfoLine = 1 << 20 // 1 MiB

// maxTotalLines caps the total number of lines scanned in
// /proc/self/mountinfo. This bounds CPU time on pathological inputs that
// might present many short malformed lines.
const maxTotalLines = MaxMounts * 10

// Mount describes a single mounted filesystem.
type Mount struct {
	// Source is the device path or pseudo-source (e.g. "/dev/disk1s5",
	// "tmpfs", "proc"). May be empty if the kernel does not expose it.
	Source string

	// DevID identifies the kernel device backing this mount as a
	// "major:minor" string (e.g. "8:1" for /dev/sda1, "0:18" for sysfs).
	// On Linux it comes from /proc/self/mountinfo field index 2; on
	// macOS it is formatted from Statfs_t.Fsid. Empty if the platform
	// does not expose a stable device identity.
	//
	// Used as the dedup key in df.filterMounts: GNU df elides
	// bind-mounts that share a device (regardless of source string),
	// keeping the entry with the shortest mount point.
	DevID string

	// MountPoint is the absolute path where the filesystem is mounted.
	MountPoint string

	// FSType is the filesystem type (e.g. "ext4", "tmpfs", "apfs").
	FSType string

	// BlockSize is the fundamental block size used by the filesystem
	// (typically 4096). All Total/Used/Free values are in bytes; this
	// field is informational.
	BlockSize uint64

	// Total is the total size of the filesystem, in bytes.
	Total uint64

	// Free is the number of bytes available to non-root users.
	Free uint64

	// Used is the number of bytes used. Computed as Total - bytes-free
	// using the kernel-reported f_blocks/f_bfree, which differs from
	// Total - Free when the filesystem reserves space for root.
	Used uint64

	// Inodes is the total number of inodes on the filesystem.
	Inodes uint64

	// InodesFree is the number of free inodes.
	InodesFree uint64

	// InodesUsed is the number of inodes in use. Inodes - InodesFree.
	InodesUsed uint64

	// Pseudo reports whether this is a pseudo / dummy filesystem
	// (tmpfs, proc, sysfs, devtmpfs, cgroup, …). The default df listing
	// hides these unless -a is given.
	Pseudo bool

	// Local reports whether the filesystem is backed by local storage
	// (i.e. not nfs, smb, fuse remote, etc.). Used by -l filtering.
	Local bool
}

// ErrNotSupported is returned by List on platforms where mount enumeration
// is not implemented (currently anything that is not Linux or macOS).
var ErrNotSupported = errors.New("not supported on this platform")

// ErrMaxMounts is returned when the mount table exceeds MaxMounts entries.
// The returned slice is truncated to MaxMounts entries.
var ErrMaxMounts = errors.New("mount table truncated: too many mounts")

// errLineTooLong is returned when a /proc/self/mountinfo line exceeds
// maxMountInfoLine bytes. Surfaced from List as a generic error.
var errLineTooLong = errors.New("mountinfo line exceeds maximum length")

// FilterFunc decides, before per-mount statfs(2) is called, whether to
// keep a mount in the listing. The argument has Source / MountPoint /
// FSType / Pseudo / Local populated from /proc/self/mountinfo; capacity
// fields are still zero. Return true to keep, false to drop.
//
// Used by df to skip filtered remote/pseudo mounts before the syscall
// is issued. Statfs(2) on a stale NFS mount can hang indefinitely and
// is not interrupted by context cancellation, so filtering up-front is
// the only way to guarantee `df -l` does not block on a dead remote.
type FilterFunc func(Mount) bool

// List enumerates the mounted filesystems on the host.
//
// On unsupported platforms it returns (nil, ErrNotSupported).
// On Linux it reads /proc/self/mountinfo, evaluates filter against each
// pre-stat Mount, and only calls statfs(2) for mounts the filter keeps.
// On macOS it calls getfsstat(2) (which is non-blocking under
// MNT_NOWAIT) and applies filter to the resulting Mounts.
//
// Pass nil for filter to keep every mount.
//
// Mounts that disappear or become inaccessible mid-enumeration are silently
// skipped; the listing is best-effort.
func List(ctx context.Context, filter FilterFunc) ([]Mount, error) {
	return listImpl(ctx, filter)
}
