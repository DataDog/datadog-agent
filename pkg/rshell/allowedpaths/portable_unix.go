// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// IsErrIsDirectory reports whether err is an "is a directory" error.
func IsErrIsDirectory(err error) bool {
	return errors.Is(err, syscall.EISDIR)
}

// FileIdentity extracts canonical file identity (dev+inode) from FileInfo.
// On Unix, this is extracted directly from Stat_t via info.Sys().
func FileIdentity(_ string, info fs.FileInfo, _ *Sandbox) (uint64, uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(st.Dev), uint64(st.Ino), true
}

// IsDevNullFile reports whether info identifies the platform's null device.
func IsDevNullFile(info fs.FileInfo) bool {
	nullInfo, err := os.Stat(os.DevNull)
	if err != nil {
		return false
	}
	dev, ino, ok := FileIdentity("", info, nil)
	nullDev, nullIno, nullOK := FileIdentity("", nullInfo, nil)
	return ok && nullOK && dev == nullDev && ino == nullIno
}

func sameOpenedRootAndPath(root *os.Root, path string) (bool, error) {
	rootInfo, err := root.Stat(".")
	if err != nil {
		return false, err
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	rootDev, rootIno, rootOK := FileIdentity("", rootInfo, nil)
	pathDev, pathIno, pathOK := FileIdentity("", pathInfo, nil)
	if !rootOK || !pathOK {
		return false, nil
	}
	return rootDev == pathDev && rootIno == pathIno, nil
}

func (r *root) accessCheck(rel string, checkRead, checkWrite, checkExec bool) (fs.FileInfo, error) {
	// Write-only or exec-only checks (no read): single Stat + mode-bit
	// inspection. No TOCTOU because there is only one resolution.
	if !checkRead {
		info, err := r.root.Stat(rel)
		if err != nil {
			return nil, err
		}
		if !effectiveHasPerm(info, false, checkWrite, checkExec) {
			return info, os.ErrPermission
		}
		return info, nil
	}

	// Read checks: open-first to get an fd, then fstat the fd.
	// O_NONBLOCK prevents blocking on FIFOs (open returns immediately
	// even without a writer). It is harmless on regular files and dirs.
	f, openErr := r.root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if openErr != nil {
		// OpenFile failed. Possible reasons:
		//   - Permission denied on a regular file (kernel/ACL)
		//   - Unopenable type (Unix socket → ENXIO/EOPNOTSUPP)
		//   - Path does not exist or symlink escape blocked
		//
		// Fall back to Stat for metadata. This is NOT a TOCTOU risk:
		// the open already failed, so there is no fd pointing to a
		// wrong inode.
		info, err := r.root.Stat(rel)
		if err != nil {
			return nil, err
		}
		// For regular files, the open failure is the kernel's
		// authoritative answer (may reflect ACLs that mode bits
		// miss). Trust it.
		if info.Mode().IsRegular() {
			return info, os.ErrPermission
		}
		// Non-regular files that can't be opened (e.g. sockets):
		// fall back to mode-bit inspection.
		if !effectiveHasPerm(info, checkRead, checkWrite, checkExec) {
			return info, os.ErrPermission
		}
		return info, nil
	}

	// OpenFile succeeded — fstat the fd for metadata from this exact inode.
	info, err := f.Stat()
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}

	// For regular files, the successful open proves read permission
	// (kernel-level, ACL-accurate). For FIFOs and directories,
	// O_NONBLOCK open succeeds regardless of read permission, so
	// mode-bit check is still needed.
	if !info.Mode().IsRegular() {
		if !effectiveHasPerm(info, checkRead, checkWrite, checkExec) {
			return info, os.ErrPermission
		}
		return info, nil
	}

	// Regular file: read proven. Check write/exec if needed.
	if checkWrite || checkExec {
		if !effectiveHasPerm(info, false, checkWrite, checkExec) {
			return info, os.ErrPermission
		}
	}
	return info, nil
}

// effectiveHasPerm checks whether the current process has the requested
// permission by inspecting the file's owner/group/other permission class
// that applies to the effective UID and GID of the running process.
//
// On Unix this uses the Stat_t from info.Sys() to determine the owning
// UID/GID and then selects the owner, group, or other permission bits
// accordingly.  If the type assertion fails (should not happen in practice),
// it falls back to checking any-class bits.
func effectiveHasPerm(info fs.FileInfo, checkRead, checkWrite, checkExec bool) bool {
	perm := info.Mode().Perm()

	// Determine which permission class applies to the current process.
	// Default to "other" bits and narrow down if we have Stat_t.
	ownerBits := fs.FileMode(0007) // other bits by default
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		uid := os.Getuid()
		if uid == 0 {
			// Root bypasses read/write permission checks (CAP_DAC_OVERRIDE).
			// Execute still requires at least one x bit to be set.
			if checkExec && perm&0111 == 0 {
				return false
			}
			return true
		}
		gid := os.Getgid()
		switch {
		case int(st.Uid) == uid:
			ownerBits = 0700
		case int(st.Gid) == gid:
			ownerBits = 0070
		default:
			ownerBits = 0007
			// Check supplementary groups — the process may belong to
			// additional groups beyond the primary GID.
			if groups, err := os.Getgroups(); err == nil {
				for _, g := range groups {
					if int(st.Gid) == g {
						ownerBits = 0070
						break
					}
				}
			}
		}
	}

	if checkRead {
		if perm&0444&ownerBits == 0 {
			return false
		}
	}
	if checkWrite {
		if perm&0222&ownerBits == 0 {
			return false
		}
	}
	if checkExec {
		if perm&0111&ownerBits == 0 {
			return false
		}
	}
	return true
}
