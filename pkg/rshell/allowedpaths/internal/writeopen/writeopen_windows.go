// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package writeopen

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func OpenRoot(*os.Root) (*os.File, error) {
	return nil, nil
}

func CloseRoot(*os.File) {}

func OpenFile(_ *os.File, root *os.Root, relPath string, flag int, perm os.FileMode) (*os.File, error) {
	// On Windows, os.Root.OpenFile uses the runtime's handle-relative
	// openat implementation with O_NOFOLLOW_ANY, which rejects reparse
	// points anywhere in the path. Keep using it here instead of the Unix
	// openat walker.
	return root.OpenFile(relPath, flag, perm)
}

// fileDispositionInformationEx mirrors the NT FILE_DISPOSITION_INFORMATION_EX
// struct (a single ULONG Flags field), which golang.org/x/sys/windows does
// not expose as a type even though it exposes the FILE_DISPOSITION_* flag
// constants and the NtSetInformationFile syscall that consumes it.
type fileDispositionInformationEx struct {
	Flags uint32
}

// Unlink removes relPath. The parent directory is opened through
// os.Root.OpenFile, reusing its handle-relative, no-follow-anywhere walk to
// validate every intermediate path component; only the final component is
// then resolved and deleted directly via NtCreateFile+NtSetInformationFile,
// mirroring what Go's os.Root.Remove does internally on Windows (which is
// unfortunately unexported and unreachable from outside the standard
// library). Doing the directory check and the delete-on-close disposition
// on the same held handle closes the TOCTOU window between a separate Lstat
// and Remove: nothing can swap the object out from under an already-open
// handle. FILE_OPEN_REPARSE_POINT ensures a symlink leaf is deleted as
// itself, never followed, matching unlink(2) semantics on Unix.
//
// The leaf is deliberately opened without FILE_NON_DIRECTORY_FILE: NTFS sets
// FILE_ATTRIBUTE_DIRECTORY on a symlink/junction whose target is a
// directory, on the reparse point itself, not just its target. With
// FILE_NON_DIRECTORY_FILE, NtCreateFile would reject opening that reparse
// point with STATUS_FILE_IS_A_DIRECTORY even though FILE_OPEN_REPARSE_POINT
// means it's the link, not a real directory, being opened — incorrectly
// refusing to remove a directory symlink. Instead, the directory check is
// done after open by inspecting the actual file attributes: an
// FILE_ATTRIBUTE_DIRECTORY without FILE_ATTRIBUTE_REPARSE_POINT is a real
// directory; a reparse point is always removable regardless of what it
// resolves to, matching Lstat-based directory detection on Unix — unless
// relPath itself syntactically demands directory semantics (a trailing
// separator, or a final "." / ".." component), in which case POSIX forces
// the target to be dereferenced (via isReparseTargetDir) before deciding
// between ErrIsDirectory and ErrNotDirectory.
func Unlink(_ *os.File, root *os.Root, relPath string) error {
	requiresDir := HasTrailingDirSyntax(relPath)

	clean := filepath.Clean(relPath)
	if clean == "." {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
	}
	base := filepath.Base(clean)
	if base == "" || base == "." || base == ".." {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
	}

	parent, err := root.OpenFile(filepath.Dir(clean), os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer parent.Close()

	nameU, err := windows.NewNTUnicodeString(base)
	if err != nil {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: err}
	}
	objAttrs := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(parent.Fd()),
		ObjectName:    nameU,
		Attributes:    windows.OBJ_CASE_INSENSITIVE,
	}

	var h windows.Handle
	iosb := &windows.IO_STATUS_BLOCK{}
	ntErr := windows.NtCreateFile(
		&h,
		windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		objAttrs,
		iosb,
		nil,
		0,
		windows.FILE_SHARE_DELETE|windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT,
		0, 0,
	)
	if ntErr != nil {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ntStatusErr(ntErr)}
	}
	defer windows.CloseHandle(h)

	var byHandleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &byHandleInfo); err != nil {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: err}
	}
	isReparsePoint := byHandleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	isRealDir := byHandleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && !isReparsePoint
	if isRealDir {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
	}

	if requiresDir {
		if isReparsePoint {
			// Trailing separator syntax forces dereferencing the link to
			// determine the real target type, matching POSIX semantics.
			targetIsDir, err := isReparseTargetDir(objAttrs)
			if err != nil {
				return &os.PathError{Op: "unlinkat", Path: relPath, Err: err}
			}
			if targetIsDir {
				return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrIsDirectory}
			}
		}
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ErrNotDirectory}
	}

	disposition := fileDispositionInformationEx{
		Flags: windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS,
	}
	if ntErr := windows.NtSetInformationFile(
		h, iosb,
		(*byte)(unsafe.Pointer(&disposition)), uint32(unsafe.Sizeof(disposition)),
		windows.FileDispositionInformationEx,
	); ntErr != nil {
		return &os.PathError{Op: "unlinkat", Path: relPath, Err: ntStatusErr(ntErr)}
	}
	return nil
}

// isReparseTargetDir opens objAttrs' object following the reparse point
// (omitting FILE_OPEN_REPARSE_POINT) to determine whether the dereferenced
// target is a directory. Used only when the caller's path syntactically
// demanded directory semantics (a trailing separator), which per POSIX
// forces dereferencing a symlink leaf rather than inspecting the link
// itself.
func isReparseTargetDir(objAttrs *windows.OBJECT_ATTRIBUTES) (bool, error) {
	var h windows.Handle
	iosb := &windows.IO_STATUS_BLOCK{}
	ntErr := windows.NtCreateFile(
		&h,
		windows.FILE_READ_ATTRIBUTES,
		objAttrs,
		iosb,
		nil,
		0,
		windows.FILE_SHARE_DELETE|windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN,
		windows.FILE_OPEN_FOR_BACKUP_INTENT,
		0, 0,
	)
	if ntErr != nil {
		return false, ntStatusErr(ntErr)
	}
	defer windows.CloseHandle(h)

	var byHandleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &byHandleInfo); err != nil {
		return false, err
	}
	return byHandleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0, nil
}

// ntStatusErr converts an NTStatus into the equivalent syscall.Errno (e.g.
// ERROR_FILE_NOT_FOUND) so callers can use errors.Is with the usual
// os.ErrNotExist / os.ErrPermission sentinels instead of matching raw
// NTStatus values.
func ntStatusErr(err error) error {
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
