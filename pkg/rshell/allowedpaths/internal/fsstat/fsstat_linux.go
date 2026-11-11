// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package fsstat

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func read(root *os.Root, relPath string, requireDirectory bool) (Info, error) {
	before, err := root.Lstat(relPath)
	if err != nil {
		return Info{}, err
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return Info{}, ErrPathChanged
	}
	if requireDirectory && !before.IsDir() {
		return Info{}, ErrNotDirectory
	}

	// O_PATH obtains a handle to the object without requiring read access or
	// opening special files for I/O. os.Root adds O_NOFOLLOW; on Linux,
	// O_PATH|O_NOFOLLOW opens a final symlink itself, which we reject below.
	f, err := root.OpenFile(relPath, unix.O_PATH, 0)
	if err != nil {
		return Info{}, err
	}
	defer f.Close() //nolint:errcheck

	opened, err := f.Stat()
	if err != nil {
		return Info{}, err
	}
	if opened.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) {
		return Info{}, ErrPathChanged
	}

	var st unix.Statfs_t
	if err := unix.Fstatfs(int(f.Fd()), &st); err != nil {
		return Info{}, err
	}

	after, err := root.Lstat(relPath)
	if err != nil {
		return Info{}, ErrPathChanged
	}
	if after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, after) {
		return Info{}, ErrPathChanged
	}

	ioBlockSize := nonnegative(int64(st.Bsize))
	fundamentalBlockSize := nonnegative(int64(st.Frsize))
	if fundamentalBlockSize == 0 {
		fundamentalBlockSize = ioBlockSize
	}

	typeID := linuxTypeID(int64(st.Type))
	return Info{
		ID:                   fsid(st.Fsid.Val[0], st.Fsid.Val[1]),
		IDAvailable:          true,
		NameMax:              nonnegative(int64(st.Namelen)),
		NameMaxAvailable:     true,
		TypeID:               typeID,
		TypeIDAvailable:      true,
		TypeName:             linuxTypeName(typeID),
		IOBlockSize:          ioBlockSize,
		FundamentalBlockSize: fundamentalBlockSize,
		Blocks:               st.Blocks,
		BlocksFree:           st.Bfree,
		BlocksAvailable:      st.Bavail,
		Files:                st.Files,
		FilesFree:            st.Ffree,
		FilesAvailable:       true,
	}, nil
}

func nonnegative(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func linuxTypeID(value int64) uint64 {
	return uint64(uint32(value))
}

func linuxTypeName(typeID uint64) string {
	switch typeID {
	case 0xadf5:
		return "adfs"
	case 0xadff:
		return "affs"
	case 0x5346414f:
		return "afs"
	case 0x187:
		return "autofs"
	case 0x9123683e:
		return "btrfs"
	case 0x27e0eb:
		return "cgroup"
	case 0x63677270:
		return "cgroup2fs"
	case 0xff534d42:
		return "cifs"
	case 0x28cd3d45:
		return "cramfs"
	case 0x64626720:
		return "debugfs"
	case 0x1cd1:
		return "devpts"
	case 0x137d:
		return "ext"
	case 0xef53:
		return "ext2/ext3"
	case 0xf2f52010:
		return "f2fs"
	case 0x65735546:
		return "fuseblk"
	case 0x4244:
		return "hfs"
	case 0x482b:
		return "hfs+"
	case 0x958458f6:
		return "hugetlbfs"
	case 0x9660:
		return "isofs"
	case 0x72b6:
		return "jffs2"
	case 0x4d44:
		return "msdos"
	case 0x6969:
		return "nfs"
	case 0x5346544e:
		return "ntfs"
	case 0x7461636f:
		return "ocfs2"
	case 0x794c7630:
		return "overlayfs"
	case 0x9fa0:
		return "proc"
	case 0x858458f6:
		return "ramfs"
	case 0x52654973:
		return "reiserfs"
	case 0x73636673:
		return "securityfs"
	case 0x73717368:
		return "squashfs"
	case 0x62656572:
		return "sysfs"
	case 0x1021994:
		return "tmpfs"
	case 0x74726163:
		return "tracefs"
	case 0x15013346:
		return "udf"
	case 0x11954:
		return "ufs"
	case 0x58465342:
		return "xfs"
	case 0x2fc12fc1:
		return "zfs"
	default:
		return fmt.Sprintf("UNKNOWN (0x%x)", typeID)
	}
}
