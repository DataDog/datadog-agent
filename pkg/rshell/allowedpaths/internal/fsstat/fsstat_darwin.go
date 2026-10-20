// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package fsstat

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// O_SEARCH is defined by Darwin as O_EXEC|O_DIRECTORY. x/sys/unix does not
// expose the composed macro.
const darwinOSearch = 0x40000000 | unix.O_DIRECTORY

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

	// O_EVTONLY avoids data I/O, but under Darwin's default process policy it
	// still carries read authorization and can return EACCES for an unreadable
	// object. O_NONBLOCK is necessary for FIFOs because Darwin otherwise waits
	// for a peer despite O_EVTONLY.
	flags := unix.O_EVTONLY
	if !before.Mode().IsRegular() {
		flags |= unix.O_NONBLOCK
	}
	// os.Root performs each path traversal relative to its pinned descriptor
	// and prevents escapes.
	f, err := root.OpenFile(relPath, flags, 0)
	if err != nil {
		if errors.Is(err, syscall.EACCES) {
			return readViaParent(root, relPath, before, err)
		}
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

	if err := revalidate(root, relPath, opened); err != nil {
		return Info{}, ErrPathChanged
	}

	return infoFromStatfs(st), nil
}

// readViaParent handles objects that cannot be opened with O_EVTONLY because
// their mode denies read access. The parent is opened for search only through
// os.Root, so no unrooted path is resolved.
func readViaParent(root *os.Root, relPath string, target os.FileInfo, openErr error) (Info, error) {
	targetDevice, ok := deviceID(target)
	if !ok {
		return Info{}, openErr
	}

	// An execute-only directory can still provide a handle tied directly to
	// the target filesystem. This also handles unreadable mount points without
	// consulting any host-wide mount table.
	if target.IsDir() {
		if info, err := readSearchDirectory(root, relPath, target); err == nil {
			return info, nil
		} else if errors.Is(err, ErrPathChanged) {
			return Info{}, ErrPathChanged
		}
	}

	parentPath := filepath.Dir(filepath.Clean(relPath))
	parent, err := root.OpenFile(parentPath, darwinOSearch, 0)
	if err != nil {
		return Info{}, err
	}
	defer parent.Close() //nolint:errcheck

	parentInfo, err := parent.Stat()
	if err != nil {
		return Info{}, err
	}
	parentDevice, ok := deviceID(parentInfo)
	if !ok {
		return Info{}, openErr
	}

	if targetDevice != parentDevice {
		// The final component is a mount boundary. Returning parent statistics
		// would describe the covered filesystem, so fail closed when the target
		// itself could not provide a metadata handle.
		return Info{}, openErr
	}

	var st unix.Statfs_t
	if err := unix.Fstatfs(int(parent.Fd()), &st); err != nil {
		return Info{}, err
	}
	// Darwin derives st_dev from the first word of the mount's FSID. Require
	// both views to agree before using the parent's statistics.
	if uint32(st.Fsid.Val[0]) != targetDevice {
		return Info{}, ErrPathChanged
	}

	if err := revalidate(root, relPath, target); err != nil {
		return Info{}, err
	}
	return infoFromStatfs(st), nil
}

func readSearchDirectory(root *os.Root, relPath string, expected os.FileInfo) (Info, error) {
	f, err := root.OpenFile(relPath, darwinOSearch, 0)
	if err != nil {
		return Info{}, err
	}
	defer f.Close() //nolint:errcheck

	opened, err := f.Stat()
	if err != nil {
		return Info{}, err
	}
	if opened.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, opened) {
		return Info{}, ErrPathChanged
	}

	var st unix.Statfs_t
	if err := unix.Fstatfs(int(f.Fd()), &st); err != nil {
		return Info{}, err
	}
	if err := revalidate(root, relPath, opened); err != nil {
		return Info{}, err
	}
	return infoFromStatfs(st), nil
}

func deviceID(info os.FileInfo) (uint32, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint32(st.Dev), true
}

func revalidate(root *os.Root, relPath string, expected os.FileInfo) error {
	after, err := root.Lstat(relPath)
	if err != nil {
		return ErrPathChanged
	}
	if after.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, after) {
		return ErrPathChanged
	}
	return nil
}

func infoFromStatfs(st unix.Statfs_t) Info {
	typeID := uint64(st.Type)
	blockSize := uint64(st.Bsize)
	return Info{
		ID:                   fsid(st.Fsid.Val[0], st.Fsid.Val[1]),
		IDAvailable:          true,
		NameMaxAvailable:     false,
		TypeID:               typeID,
		TypeIDAvailable:      true,
		TypeName:             unix.ByteSliceToString(st.Fstypename[:]),
		IOBlockSize:          blockSize,
		FundamentalBlockSize: blockSize,
		Blocks:               st.Blocks,
		BlocksFree:           st.Bfree,
		BlocksAvailable:      st.Bavail,
		Files:                st.Files,
		FilesFree:            st.Ffree,
		FilesAvailable:       true,
	}
}
