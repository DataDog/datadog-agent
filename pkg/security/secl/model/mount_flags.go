// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package model

import (
	"strings"

	"golang.org/x/sys/unix"
)

// Canonical per-mount attribute bits. This is the normalized representation of
// the security-relevant mount flags, adopting the stable UAPI MOUNT_ATTR_*
// layout so that values coming from statmount(2) map through unchanged. eBPF
// (vfsmount MNT_*) and procfs option strings are translated into this layout so
// the same mount always yields the same value regardless of the source that
// observed it. See pkg/security/security_profile/workload_mounts.md.
const (
	MountAttrReadOnly    = uint32(unix.MOUNT_ATTR_RDONLY)
	MountAttrNoSUID      = uint32(unix.MOUNT_ATTR_NOSUID)
	MountAttrNoDev       = uint32(unix.MOUNT_ATTR_NODEV)
	MountAttrNoExec      = uint32(unix.MOUNT_ATTR_NOEXEC)
	MountAttrNoSymFollow = uint32(unix.MOUNT_ATTR_NOSYMFOLLOW)

	mountAttrMask = MountAttrReadOnly | MountAttrNoSUID | MountAttrNoDev | MountAttrNoExec | MountAttrNoSymFollow
)

// Raw vfsmount mnt_flags bits (kernel include/linux/mount.h). These are not
// exposed by golang.org/x/sys/unix because they are kernel-internal.
const (
	mntNoSUID      = 0x01
	mntNoDev       = 0x02
	mntNoExec      = 0x04
	mntReadOnly    = 0x40
	mntNoSymFollow = 0x80
)

// NormalizeMountFlagsFromVFS converts raw vfsmount mnt_flags (as read from the
// kernel by eBPF) into the canonical per-mount attribute bitmask.
func NormalizeMountFlagsFromVFS(mntFlags uint32) uint32 {
	var flags uint32
	if mntFlags&mntReadOnly != 0 {
		flags |= MountAttrReadOnly
	}
	if mntFlags&mntNoSUID != 0 {
		flags |= MountAttrNoSUID
	}
	if mntFlags&mntNoDev != 0 {
		flags |= MountAttrNoDev
	}
	if mntFlags&mntNoExec != 0 {
		flags |= MountAttrNoExec
	}
	if mntFlags&mntNoSymFollow != 0 {
		flags |= MountAttrNoSymFollow
	}
	return flags
}

// NormalizeMountFlagsFromAttr masks a statmount(2) mnt_attr value down to the
// canonical per-mount attribute bitmask.
func NormalizeMountFlagsFromAttr(attr uint64) uint32 {
	return uint32(attr) & mountAttrMask
}

// NormalizeMountFlagsFromOptions converts a comma-separated procfs mountinfo
// options string (e.g. "ro,nosuid,nodev") into the canonical per-mount
// attribute bitmask.
func NormalizeMountFlagsFromOptions(options string) uint32 {
	var flags uint32
	for opt := range strings.SplitSeq(options, ",") {
		switch opt {
		case "ro":
			flags |= MountAttrReadOnly
		case "nosuid":
			flags |= MountAttrNoSUID
		case "nodev":
			flags |= MountAttrNoDev
		case "noexec":
			flags |= MountAttrNoExec
		case "nosymfollow":
			flags |= MountAttrNoSymFollow
		}
	}
	return flags
}
