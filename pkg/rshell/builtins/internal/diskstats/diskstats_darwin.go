// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package diskstats

import (
	"context"
	"fmt"

	"golang.org/x/sys/unix"
)

// darwinPseudoTypes lists the macOS filesystem-type names that GNU df
// classifies as pseudo / dummy. Mirrors the Linux table at the top of
// diskstats_linux.go, but uses the macOS spelling (e.g. "devfs", "autofs").
var darwinPseudoTypes = map[string]bool{
	"autofs": true,
	"devfs":  true,
	"fdesc":  true,
	"kernfs": true,
	"map":    true, // map auto_home, map -hosts
	"none":   true,
	"procfs": true,
}

// darwinRemoteTypes lists macOS network filesystem-type names.
var darwinRemoteTypes = map[string]bool{
	"nfs":    true,
	"smbfs":  true,
	"afpfs":  true,
	"webdav": true,
}

// listImpl enumerates macOS mounts via getfsstat(2). The MNT_NOWAIT flag
// avoids blocking on remote filesystems that are temporarily unavailable,
// so the filter argument is applied as a post-filter (cosmetic on Darwin)
// rather than as a hang-prevention measure (essential on Linux).
func listImpl(ctx context.Context, filter FilterFunc) ([]Mount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Size the buffer up front so we do not have to retry on growth.
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}
	truncated := false
	if n > MaxMounts {
		n = MaxMounts
		truncated = true
	}

	bufs := make([]unix.Statfs_t, n)
	got, err := unix.Getfsstat(bufs, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if got > n {
		got = n
	}

	out := make([]Mount, 0, got)
	for i := range got {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		st := bufs[i]
		fsType := unix.ByteSliceToString(st.Fstypename[:])
		mp := unix.ByteSliceToString(st.Mntonname[:])
		src := unix.ByteSliceToString(st.Mntfromname[:])

		bsize := uint64(st.Bsize)
		if bsize == 0 {
			bsize = 1
		}

		// Saturating multiply guards against a buggy filesystem
		// reporting block counts above MaxUint64/bsize.
		used := mulSat(subSat(uint64(st.Blocks), uint64(st.Bfree)), bsize)
		inodesUsed := subSat(uint64(st.Files), uint64(st.Ffree))

		pseudo := darwinPseudoTypes[fsType]
		// MNT_LOCAL=0 marks both remote mounts and pseudo filesystems
		// (devfs, autofs, …); subtracting the pseudo set isolates the
		// actually-remote ones.
		remote := darwinRemoteTypes[fsType] || (st.Flags&uint32(unix.MNT_LOCAL) == 0 && !pseudo)
		// DevID from Fsid (a [2]int32 unique per filesystem).
		devID := fmt.Sprintf("%d:%d", st.Fsid.Val[0], st.Fsid.Val[1])
		m := Mount{
			Source:     src,
			DevID:      devID,
			MountPoint: mp,
			FSType:     fsType,
			BlockSize:  bsize,
			Total:      mulSat(uint64(st.Blocks), bsize),
			Free:       mulSat(uint64(st.Bavail), bsize),
			Used:       used,
			Inodes:     uint64(st.Files),
			InodesFree: uint64(st.Ffree),
			InodesUsed: inodesUsed,
			Pseudo:     pseudo,
			// "Local" means "not remote" per GNU df. Pseudo mounts
			// (devfs, autofs, …) are local — they live in kernel
			// memory, not on a remote server.
			Local: !remote,
		}
		if filter != nil && !filter(m) {
			continue
		}
		out = append(out, m)
	}
	if truncated {
		return out, ErrMaxMounts
	}
	return out, nil
}
