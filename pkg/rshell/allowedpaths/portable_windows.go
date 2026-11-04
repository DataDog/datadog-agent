// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// IsErrIsDirectory checks if the error is the Windows equivalent of EISDIR.
// Two distinct Windows errors surface here depending on how the directory
// was opened:
//   - Opening a directory O_RDONLY through os.Root.OpenFile succeeds (no
//     FILE_NON_DIRECTORY_FILE option is set for read access), and the
//     failure only appears later, when Read is attempted on the directory
//     handle: ERROR_INVALID_FUNCTION (errno 1).
//   - Opening a directory O_WRONLY/O_RDWR — as write-target resolution
//     does for logrotate/truncate/rm-style operations — sets
//     FILE_NON_DIRECTORY_FILE, so NtCreateFile itself fails with
//     STATUS_FILE_IS_A_DIRECTORY, which Go's runtime maps to the
//     synthetic syscall.EISDIR value at open time.
func IsErrIsDirectory(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.Errno(1) || errno == syscall.EISDIR // ERROR_INVALID_FUNCTION or EISDIR
	}
	return false
}

// FileIdentity extracts canonical file identity on Windows using
// GetFileInformationByHandle (volume serial + file index).
// The path and sandbox are needed to open the file through the sandbox.
func FileIdentity(absPath string, _ fs.FileInfo, sandbox *Sandbox) (uint64, uint64, bool) {
	ar, relPath, ok := sandbox.resolve(absPath)
	if !ok {
		return 0, 0, false
	}
	f, err := ar.root.OpenFile(relPath, os.O_RDONLY, 0)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	volume, index, err := fileIdentityFromHandle(f)
	if err != nil {
		return 0, 0, false
	}
	return volume, index, true
}

func sameOpenedRootAndPath(root *os.Root, path string) (bool, error) {
	rootFile, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer rootFile.Close()

	pathRoot, err := os.OpenRoot(path)
	if err != nil {
		return false, err
	}
	defer pathRoot.Close()

	pathFile, err := pathRoot.Open(".")
	if err != nil {
		return false, err
	}
	defer pathFile.Close()

	rootVolume, rootIndex, err := fileIdentityFromHandle(rootFile)
	if err != nil {
		return false, err
	}
	pathVolume, pathIndex, err := fileIdentityFromHandle(pathFile)
	if err != nil {
		return false, err
	}
	return rootVolume == pathVolume && rootIndex == pathIndex, nil
}

func fileIdentityFromHandle(f *os.File) (uint64, uint64, error) {
	h := syscall.Handle(f.Fd())
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &d); err != nil {
		return 0, 0, err
	}
	return uint64(d.VolumeSerialNumber), uint64(d.FileIndexHigh)<<32 | uint64(d.FileIndexLow), nil
}

// accessCheck verifies the path is inside the sandbox via os.Root.Stat,
// then checks read permission by attempting to open the file through
// os.Root. This respects NTFS ACLs — the kernel denies the open if
// the current user lacks read permission. Named pipes cannot appear in
// regular directories on Windows, so this cannot block.
//
//   - Read: verified by opening through os.Root (respects NTFS ACLs).
//   - Write: checked via mode bits from Stat. On Windows,
//     FILE_ATTRIBUTE_READONLY clears the write permission bits in
//     Mode().Perm(), so mode-bit inspection is reliable.
//   - Execute: Windows has no POSIX execute bits. The check always
//     returns ErrPermission so that test -x behaves like a POSIX shell.
func (r *root) accessCheck(rel string, checkRead, checkWrite, checkExec bool) (fs.FileInfo, error) {
	info, err := r.root.Stat(rel)
	if err != nil {
		return nil, err
	}

	// Windows has no POSIX execute bits — always deny execute checks.
	if checkExec {
		return info, os.ErrPermission
	}

	// On Windows, FILE_ATTRIBUTE_READONLY clears the write permission
	// bits in Mode().Perm(). Check them for write access.
	if checkWrite && info.Mode().Perm()&0200 == 0 {
		return info, os.ErrPermission
	}

	if checkRead && !info.IsDir() {
		f, err := r.root.OpenFile(rel, os.O_RDONLY, 0)
		if err != nil {
			return info, os.ErrPermission
		}
		if err := f.Close(); err != nil {
			return info, err
		}
	}

	return info, nil
}
