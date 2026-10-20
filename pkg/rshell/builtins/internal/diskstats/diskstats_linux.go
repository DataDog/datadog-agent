// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package diskstats

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// mountInfoPath is the kernel pseudo-file enumerated by listImpl. It is
// hardcoded — never derived from user input — so it is exempt from the
// AllowedPaths sandbox.
const mountInfoPath = "/proc/self/mountinfo"

// pseudoTypes lists kernel-memory filesystem types that are hidden from the
// default listing, matching the classification `df` implementations
// commonly apply so that scripts see only filesystems backing real storage.
//
// Several types are intentionally NOT classified as pseudo even though
// they live in kernel memory:
//
//   - "overlay": the default root filesystem inside Docker / Kubernetes
//     containers, which represents the user's actual storage. Hiding it
//     would make `df` print only the header on a typical container host.
//   - "tmpfs", "devtmpfs": RAM-backed but report real, useful capacity
//     (think /dev/shm or /run). GNU df lists nonzero tmpfs mounts in
//     the default output; hiding them would make scripts that watch
//     shared-memory or run-state usage fail silently.
//   - "squashfs": real read-only filesystem (snap packages, AppImage,
//     live CDs). Reports real on-disk usage. GNU df does not classify
//     it as dummy; hiding it would make snap loop mounts disappear
//     from `df` and `df -t squashfs` on a typical Ubuntu desktop.
var pseudoTypes = map[string]bool{
	"autofs":          true,
	"binfmt_misc":     true,
	"bpf":             true,
	"cgroup":          true,
	"cgroup2":         true,
	"configfs":        true,
	"debugfs":         true,
	"devfs":           true,
	"devpts":          true,
	"efivarfs":        true,
	"fuse.gvfsd-fuse": true,
	"fuse.portal":     true,
	"fusectl":         true,
	"hugetlbfs":       true,
	"mqueue":          true,
	"none":            true,
	"nsfs":            true,
	"proc":            true,
	"pstore":          true,
	"ramfs":           true,
	"rpc_pipefs":      true,
	"securityfs":      true,
	"selinuxfs":       true,
	"sysfs":           true,
	"tracefs":         true,
}

// remoteTypePrefixes lists filesystem-type prefixes that mark a filesystem
// as remote (i.e. !Local), matching the classification GNU df applies.
//
// Linux mountinfo reports FUSE mounts as "fuse.<subtype>" (e.g.
// "fuse.sshfs", "fuse.smbnetfs"), so the remote FUSE backends are
// listed under their full "fuse." prefix here in addition to their
// short forms. A bare "sshfs" prefix would not match "fuse.sshfs"
// because HasPrefix is anchored at byte zero. Missing this means
// `df -l` can still call statfs(2) on a stale sshfs mount and hang,
// so the explicit "fuse.*" entries are load-bearing for the documented
// pre-stat hang protection.
var remoteTypePrefixes = []string{
	"nfs",
	"cifs",
	"smb",
	"afs",
	"ceph",
	"glusterfs",
	"sshfs",
	"davfs",
	// FUSE subtypes: anything in fuse.<remote-backend> form.
	"fuse.sshfs",
	"fuse.smb",
	"fuse.cifs",
	"fuse.davfs",
	"fuse.glusterfs",
	"fuse.cephfs",
	"fuse.nfs",
	"fuse.s3",
	"fuse.rclone",
}

// listImpl enumerates Linux mounts.
//
// It reads /proc/self/mountinfo (sandbox-exempt; the path is hardcoded),
// parses each line into a Mount, evaluates the caller's filter against
// the pre-stat Mount, and only then calls statfs(2) on the kept mounts.
// Filtering before statfs is critical: statfs(2) on a stale NFS or CIFS
// mount can block indefinitely and is not interrupted by context
// cancellation, so `df -l` would otherwise hang on dead remotes.
//
// Mounts that fail statfs (transient EACCES/ENOENT, race with umount)
// are silently skipped.
func listImpl(ctx context.Context, filter FilterFunc) ([]Mount, error) {
	f, err := os.Open(mountInfoPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mountInfoPath, err)
	}
	defer f.Close() //nolint:errcheck

	mounts, parseErr := parseMountInfo(ctx, f)
	if parseErr != nil && !errors.Is(parseErr, ErrMaxMounts) {
		return nil, parseErr
	}

	out := make([]Mount, 0, len(mounts))
	for i := range mounts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m := mounts[i]
		if filter != nil && !filter(m) {
			continue
		}
		var st unix.Statfs_t
		if err := unix.Statfs(m.MountPoint, &st); err != nil {
			// Skip mounts that disappear or become inaccessible
			// between the mountinfo read and the statfs call.
			continue
		}
		// Per statfs(2): f_blocks/f_bfree/f_bavail are counted in
		// f_frsize units (the fragment size); f_bsize is only the
		// optimal transfer block size. They are usually equal, but
		// can differ on FUSE-backed mounts — using Bsize there would
		// scale the reported sizes incorrectly. Match GNU coreutils:
		// prefer Frsize when non-zero, fall back to Bsize otherwise.
		bsize := uint64(st.Frsize)
		if bsize == 0 {
			bsize = uint64(st.Bsize)
		}
		if bsize == 0 {
			bsize = 1
		}
		m.BlockSize = bsize
		// Saturating multiply: a buggy/malicious FUSE FS could
		// report block counts above MaxUint64/bsize, which would
		// wrap a plain a*b. Saturating keeps the displayed values
		// monotonic and prevents one rogue mount from corrupting
		// the --total accumulation.
		m.Total = mulSat(uint64(st.Blocks), bsize)
		m.Free = mulSat(uint64(st.Bavail), bsize)
		// Used is computed from f_blocks - f_bfree (root-reserved
		// blocks are counted as used), which differs from Total - Free.
		m.Used = mulSat(subSat(uint64(st.Blocks), uint64(st.Bfree)), bsize)
		m.Inodes = uint64(st.Files)
		m.InodesFree = uint64(st.Ffree)
		m.InodesUsed = subSat(uint64(st.Files), uint64(st.Ffree))
		out = append(out, m)
	}
	return out, parseErr
}

// parseMountInfo reads /proc/self/mountinfo from r and returns one Mount
// per line. Block/inode fields are left zero — the caller fills them via
// statfs(2). Returns ErrMaxMounts when the table is truncated and
// errLineTooLong when a line exceeds maxMountInfoLine.
func parseMountInfo(ctx context.Context, r io.Reader) ([]Mount, error) {
	mounts := make([]Mount, 0, 64)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4096), maxMountInfoLine)

	totalLines := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return mounts, err
		}
		totalLines++
		if totalLines > maxTotalLines {
			return mounts, fmt.Errorf("mountinfo: scanned more than %d lines", maxTotalLines)
		}
		if len(mounts) >= MaxMounts {
			return mounts, ErrMaxMounts
		}
		line := scanner.Text()
		m, ok := parseMountInfoLine(line)
		if !ok {
			continue
		}
		mounts = append(mounts, m)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return mounts, errLineTooLong
		}
		return mounts, err
	}
	return mounts, nil
}

// parseMountInfoLine parses a single /proc/self/mountinfo line.
//
// The format is:
//
//	mount_id parent_id major:minor root mount_point mount_opts [opt_fields...] - fstype source super_opts
//
// The optional-fields section is variable-length and terminated by a
// literal " - " separator (a single hyphen as its own field). Fields after
// that separator are: filesystem type, mount source, super options.
//
// Returns ok=false on malformed input rather than an error so the caller
// can skip and continue.
func parseMountInfoLine(line string) (Mount, bool) {
	// Locate the " - " separator. It is always surrounded by single
	// space characters, and a literal "-" never appears as an
	// independent field before it (paths/options can contain "-" but
	// they are escaped or run together with other characters in their
	// own field).
	pre, post, ok := strings.Cut(line, " - ")
	if !ok {
		return Mount{}, false
	}

	preFields := strings.Fields(pre)
	if len(preFields) < 6 {
		return Mount{}, false
	}
	postFields := strings.Fields(post)
	if len(postFields) < 2 {
		// Need at least fstype and source.
		return Mount{}, false
	}

	devID := preFields[2] // mountinfo field 2 is "major:minor"
	mountPoint := unescapeMountField(preFields[4])
	fsType := postFields[0]
	source := unescapeMountField(postFields[1])

	pseudo := pseudoTypes[fsType]
	// "Local" means "not remote" per GNU df: pseudo mounts (proc,
	// sysfs, cgroup, …) are local in this sense — they live in
	// kernel memory, not on a remote server. This matters for
	// `df -al`: GNU includes local pseudo mounts when -a re-enables
	// them, and -l only filters out actually-remote (NFS / CIFS /
	// fuse.sshfs) entries.
	local := !isRemoteSource(source, fsType)

	return Mount{
		Source:     source,
		DevID:      devID,
		MountPoint: mountPoint,
		FSType:     fsType,
		Pseudo:     pseudo,
		Local:      local,
	}, true
}

// isRemoteSource reports whether a mount is remote / network-backed.
//
// A mount is treated as remote if its source name carries a network
// signature OR its type is in the explicit remote-type list. Checking the
// source is
// load-bearing because some remote mounts surface under generic types
// (e.g. NFS reported as "nfs4" without our prefix matching, gpfs/acfs,
// or "auto" mounts), but the source string still encodes the network
// origin:
//   - "host:/export" → NFS
//   - "//server/share" → SMB / CIFS
//   - "user@host:/path" → sshfs / autofs-mounted SSHFS
//
// Without this check `df -l` would silently keep stale remote mounts
// and listImpl would still call statfs(2) on them, defeating the
// documented pre-stat hang protection.
func isRemoteSource(source, fsType string) bool {
	if strings.HasPrefix(source, "//") {
		return true
	}
	if strings.Contains(source, ":") {
		return true
	}
	for _, p := range remoteTypePrefixes {
		if strings.HasPrefix(fsType, p) {
			return true
		}
	}
	return false
}

// unescapeMountField undoes the octal escapes that the kernel applies to
// space (\040), tab (\011), newline (\012), and backslash (\134) in
// mountinfo paths.
func unescapeMountField(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
			v := (int(s[i+1]-'0') << 6) | (int(s[i+2]-'0') << 3) | int(s[i+3]-'0')
			b.WriteByte(byte(v))
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(b byte) bool { return b >= '0' && b <= '7' }
