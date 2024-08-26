// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// Code in this file was blatantly taken from
// https://go.dev/src/internal/syscall/windows/reparse_windows.go
// https://go.dev/src/internal/syscall/windows/security_windows.go
// https://go.dev/src/os/os_windows_test.go
// and adapted.

//go:build windows

package repository

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	//nolint:revive // keep original name.
	SYMLINK_FLAG_RELATIVE = 1
)

// REPARSE_DATA_BUFFER_HEADER is a common part of REPARSE_DATA_BUFFER structure.
//
//nolint:revive // keep original name.
type REPARSE_DATA_BUFFER_HEADER struct {
	ReparseTag uint32
	// The size, in bytes, of the reparse data that follows
	// the common portion of the REPARSE_DATA_BUFFER element.
	// This value is the length of the data starting at the
	// SubstituteNameOffset field.
	ReparseDataLength uint16
	Reserved          uint16
}

type symbolicLinkReparseBuffer struct {
	// The integer that contains the offset, in bytes,
	// of the substitute name string in the PathBuffer array,
	// computed as an offset from byte 0 of PathBuffer. Note that
	// this offset must be divided by 2 to get the array index.
	SubstituteNameOffset uint16
	// The integer that contains the length, in bytes, of the
	// substitute name string. If this string is null-terminated,
	// SubstituteNameLength does not include the Unicode null character.
	SubstituteNameLength uint16
	// PrintNameOffset is similar to SubstituteNameOffset.
	PrintNameOffset uint16
	// PrintNameLength is similar to SubstituteNameLength.
	PrintNameLength uint16
	// Flags specifies whether the substitute name is a full path name or
	// a path name relative to the directory containing the symbolic link.
	Flags      uint32
	PathBuffer [1]uint16
}

// reparseData is used to build reparse buffer data required for tests.
type reparseData struct {
	substituteName namePosition
	printName      namePosition
	pathBuf        []uint16
}

type namePosition struct {
	offset uint16
	length uint16
}

func (rd *reparseData) addUTF16s(s []uint16) (offset uint16) {
	off := len(rd.pathBuf) * 2
	rd.pathBuf = append(rd.pathBuf, s...)
	return uint16(off)
}

func (rd *reparseData) addString(s string) (offset, length uint16) {
	p, _ := syscall.UTF16FromString(s)
	return rd.addUTF16s(p), uint16(len(p)-1) * 2 // do not include terminating NUL in the length (as per PrintNameLength and SubstituteNameLength documentation)
}

func (rd *reparseData) addSubstituteName(name string) {
	rd.substituteName.offset, rd.substituteName.length = rd.addString(name)
}

func (rd *reparseData) addPrintName(name string) {
	rd.printName.offset, rd.printName.length = rd.addString(name)
}

// pathBuffeLen returns length of rd pathBuf in bytes.
func (rd *reparseData) pathBuffeLen() uint16 {
	return uint16(len(rd.pathBuf)) * 2
}

// Windows REPARSE_DATA_BUFFER contains union member, and cannot be
// translated into Go directly. _REPARSE_DATA_BUFFER type is to help
// construct alternative versions of Windows REPARSE_DATA_BUFFER with
// union part of symbolicLinkReparseBuffer or MountPointReparseBuffer type.
//
//nolint:revive // keep original name.
type _REPARSE_DATA_BUFFER struct {
	header REPARSE_DATA_BUFFER_HEADER
	detail [syscall.MAXIMUM_REPARSE_DATA_BUFFER_SIZE]byte
}

func createDirLink(link string, rdb *_REPARSE_DATA_BUFFER) error {
	linkp, err := syscall.UTF16FromString(link)
	if err != nil {
		return err
	}

	fd, err := syscall.CreateFile(&linkp[0], syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(fd)

	buflen := uint32(rdb.header.ReparseDataLength) + uint32(unsafe.Sizeof(rdb.header))
	var bytesReturned uint32
	return syscall.DeviceIoControl(fd, windows.FSCTL_SET_REPARSE_POINT,
		(*byte)(unsafe.Pointer(&rdb.header)), buflen, nil, 0, &bytesReturned, nil)
}

func createSymbolicLink(link string, target *reparseData, isrelative bool) error {
	var buf *symbolicLinkReparseBuffer
	buflen := uint16(unsafe.Offsetof(buf.PathBuffer)) + target.pathBuffeLen() // see ReparseDataLength documentation
	byteblob := make([]byte, buflen)
	buf = (*symbolicLinkReparseBuffer)(unsafe.Pointer(&byteblob[0]))
	buf.SubstituteNameOffset = target.substituteName.offset
	buf.SubstituteNameLength = target.substituteName.length
	buf.PrintNameOffset = target.printName.offset
	buf.PrintNameLength = target.printName.length
	if isrelative {
		buf.Flags = SYMLINK_FLAG_RELATIVE
	}
	pbuflen := len(target.pathBuf)
	copy((*[2048]uint16)(unsafe.Pointer(&buf.PathBuffer[0]))[:pbuflen:pbuflen], target.pathBuf)

	var rdb _REPARSE_DATA_BUFFER
	rdb.header.ReparseTag = syscall.IO_REPARSE_TAG_SYMLINK
	rdb.header.ReparseDataLength = buflen
	copy(rdb.detail[:], byteblob)

	return createDirLink(link, &rdb)
}

func setReparsePoint(target, link string) error {
	var t reparseData
	t.addPrintName(target)
	t.addSubstituteName(`\??\` + target)
	// isrelative is always false here because the paths in the installer are all absolute.
	// Still, we keep the flag present in case that would change.
	return createSymbolicLink(link, &t, false)
}

func symlinkWithImpersonation(target, link string, fn func(target, link string) error) error {
	// The ImpersonateSelf function obtains an access token that impersonates the security context of the calling process.
	// The token is assigned to the calling thread, so we need to lock the current goroutine to the OS thread where
	// we impersonate.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := windows.ImpersonateSelf(windows.SecurityImpersonation)
	if err != nil {
		return err
	}

	defer func() { _ = windows.RevertToSelf() }()

	err = enableCurrentThreadPrivilege("SeCreateSymbolicLinkPrivilege")
	if err != nil {
		return fmt.Errorf(`could not enable "SeCreateSymbolicLinkPrivilege": %v`, err)
	}

	return fn(target, link)
}

func enableCurrentThreadPrivilege(privilegeName string) error {
	ct := windows.CurrentThread()
	var t windows.Token
	err := windows.OpenThreadToken(ct, syscall.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES, false, &t)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(syscall.Handle(t))

	var tp windows.Tokenprivileges

	privStr, err := syscall.UTF16PtrFromString(privilegeName)
	if err != nil {
		return err
	}
	err = windows.LookupPrivilegeValue(nil, privStr, &tp.Privileges[0].Luid)
	if err != nil {
		return err
	}
	tp.PrivilegeCount = 1
	tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
	return windows.AdjustTokenPrivileges(t, false, &tp, 0, nil, nil)
}

// atomicSymlink wraps os.Symlink, replacing an existing symlink with the same name.
// This function will also request the SeCreateSymbolicLinkPrivilege when the link already exists, so make sure to take
// into account the considerations for that privilege:
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/create-symbolic-links
func atomicSymlink(oldname, newname string) error {
	// Fast path: same as linux, if newname does not exist yet, we can skip the whole dance
	// below.
	if err := os.Symlink(oldname, newname); err == nil || !os.IsExist(err) {
		return err
	}
	// symlinkWithImpersonation accepts a function because that allowed me to
	// quickly test various algorithm for replacing the existing link by running them
	// with and without impersonation.
	return symlinkWithImpersonation(oldname, newname, setReparsePoint)
}
