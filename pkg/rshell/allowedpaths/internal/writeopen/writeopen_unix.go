// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package writeopen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func OpenRoot(root *os.Root) (*os.File, error) {
	return root.Open(".")
}

func CloseRoot(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}

func OpenFile(rootFile *os.File, _ *os.Root, relPath string, flag int, perm os.FileMode) (*os.File, error) {
	if rootFile == nil {
		return nil, &os.PathError{Op: "openat", Path: relPath, Err: os.ErrPermission}
	}

	clean := filepath.Clean(relPath)
	if clean == "." {
		return nil, &os.PathError{Op: "openat", Path: relPath, Err: unix.EISDIR}
	}
	components := strings.Split(clean, string(filepath.Separator))
	dirFD := int(rootFile.Fd())
	closeDir := false
	for _, component := range components[:len(components)-1] {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			if closeDir {
				_ = unix.Close(dirFD)
			}
			return nil, &os.PathError{Op: "openat", Path: relPath, Err: os.ErrPermission}
		}
		nextFD, err := unix.Openat(dirFD, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if closeDir {
			_ = unix.Close(dirFD)
		}
		if err != nil {
			return nil, writePathError(relPath, err)
		}
		dirFD = nextFD
		closeDir = true
	}

	base := components[len(components)-1]
	if base == "" || base == "." || base == ".." {
		if closeDir {
			_ = unix.Close(dirFD)
		}
		return nil, &os.PathError{Op: "openat", Path: relPath, Err: unix.EISDIR}
	}
	fd, err := unix.Openat(dirFD, base, flag|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(perm.Perm()))
	if closeDir {
		_ = unix.Close(dirFD)
	}
	if err != nil {
		return nil, writePathError(relPath, err)
	}
	f := os.NewFile(uintptr(fd), relPath)
	if f == nil {
		_ = unix.Close(fd)
		return nil, &os.PathError{Op: "openat", Path: relPath, Err: errors.New("invalid file descriptor")}
	}
	return f, nil
}

func writePathError(relPath string, err error) error {
	switch {
	case errors.Is(err, unix.ELOOP):
		err = ErrSymlinkWriteTarget
	case errors.Is(err, unix.ENXIO):
		// See ErrNotRegularFile: ENXIO on a write open always means the
		// target is not a regular file (readerless FIFO, or a device node
		// with no backing device). Remapping it keeps the message stable
		// and platform-independent. os.ErrNotExist is deliberately left
		// untouched — truncate -c relies on errors.Is against it.
		err = ErrNotRegularFile
	}
	return &os.PathError{Op: "openat", Path: relPath, Err: err}
}

// Unlink removes relPath via an atomic, no-follow openat walk to the parent
// directory followed by unlinkat on the held directory fd. Intermediate
// components are rejected if they are symlinks (same as OpenFile); the final
// component may be a symlink, since unlink(2) removes the link itself
// without following it — unless relPath itself syntactically demands
// directory semantics (a trailing separator, or a final "." / ".."
// component, e.g. "file/" or "symlink-to-file/"), in which case POSIX forces
// the target to be dereferenced before the directory check. Directories are
// rejected via fstatat on the same held fd, closing the TOCTOU window
// between a directory check and the actual removal.
func Unlink(rootFile *os.File, _ *os.Root, relPath string) error {
	if rootFile == nil {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: os.ErrPermission}
	}

	requiresDir := HasTrailingDirSyntax(relPath)

	clean := filepath.Clean(relPath)
	if clean == "." {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
	}
	components := strings.Split(clean, string(filepath.Separator))
	dirFD := int(rootFile.Fd())
	closeDir := false
	for _, component := range components[:len(components)-1] {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			if closeDir {
				_ = unix.Close(dirFD)
			}
			return &os.PathError{Op: "unlinkat", Path: relPath, Err: os.ErrPermission}
		}
		nextFD, err := unix.Openat(dirFD, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if closeDir {
			_ = unix.Close(dirFD)
		}
		if err != nil {
			return writePathError(relPath, err)
		}
		dirFD = nextFD
		closeDir = true
	}
	if closeDir {
		defer func() { _ = unix.Close(dirFD) }()
	}

	base := components[len(components)-1]
	if base == "" || base == "." || base == ".." {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
	}

	statFlags := unix.AT_SYMLINK_NOFOLLOW
	if requiresDir {
		statFlags = 0
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(dirFD, base, &stat, statFlags); err != nil {
		return writePathError(relPath, err)
	}
	if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
	}
	if requiresDir {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrNotDirectory}
	}

	if err := unix.Unlinkat(dirFD, base, 0); err != nil {
		return writePathError(relPath, err)
	}
	return nil
}
