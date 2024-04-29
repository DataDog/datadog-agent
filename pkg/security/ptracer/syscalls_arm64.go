// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package ptracer

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	OpenatNr         = unix.SYS_OPENAT            // OpenatNr defines the syscall ID for arm64
	Openat2Nr        = unix.SYS_OPENAT2           // Openat2Nr defines the syscall ID for amd64
	NameToHandleAtNr = unix.SYS_NAME_TO_HANDLE_AT // NameToHandleAtNr defines the syscall ID for amd64
	OpenByHandleAtNr = unix.SYS_OPEN_BY_HANDLE_AT // OpenByHandleAtNr defines the syscall ID for amd64
	ExecveNr         = unix.SYS_EXECVE            // ExecveNr defines the syscall ID for arm64
	ExecveatNr       = unix.SYS_EXECVEAT          // ExecveatNr defines the syscall ID for arm64
	CloneNr          = unix.SYS_CLONE             // CloneNr defines the syscall ID for arm64
	Clone3Nr         = unix.SYS_CLONE3            // Clone3Nr defines the syscall ID for arm64
	ExitNr           = unix.SYS_EXIT              // ExitNr defines the syscall ID for arm64
	FcntlNr          = unix.SYS_FCNTL             // FcntlNr defines the syscall ID for arm64
	DupNr            = unix.SYS_DUP               // DupNr defines the syscall ID for arm64
	Dup3Nr           = unix.SYS_DUP3              // Dup3Nr defines the syscall ID for arm64
	ChdirNr          = unix.SYS_CHDIR             // ChdirNr defines the syscall ID for arm64
	FchdirNr         = unix.SYS_FCHDIR            // FchdirNr defines the syscall ID for arm64
	SetuidNr         = unix.SYS_SETUID            // SetuidNr defines the syscall ID for arm64
	SetgidNr         = unix.SYS_SETGID            // SetgidNr defines the syscall ID for arm64
	SetreuidNr       = unix.SYS_SETREUID          // SetreuidNr defines the syscall ID for arm64
	SetregidNr       = unix.SYS_SETREGID          // SetregidNr defines the syscall ID for arm64
	SetresuidNr      = unix.SYS_SETRESUID         // SetresuidNr defines the syscall ID for arm64
	SetresgidNr      = unix.SYS_SETRESGID         // SetresgidNr defines the syscall ID for arm64
	SetfsuidNr       = unix.SYS_SETFSUID          // SetfsuidNr defines the syscall ID for arm64
	SetfsgidNr       = unix.SYS_SETFSGID          // SetfsgidNr defines the syscall ID for arm64
	CloseNr          = unix.SYS_CLOSE             // CloseNr defines the syscall ID for arm64
	MemfdCreateNr    = unix.SYS_MEMFD_CREATE      // MemfdCreateNr defines the syscall ID for arm64
	CapsetNr         = unix.SYS_CAPSET            // CapsetNr defines the syscall ID for arm64
	UnlinkatNr       = unix.SYS_UNLINKAT          // UnlinkatNr defines the syscall ID for arm64
	RenameAtNr       = unix.SYS_RENAMEAT          // RenameAtNr defines the syscall ID for arm64
	RenameAt2Nr      = unix.SYS_RENAMEAT2         // RenameAt2Nr defines the syscall ID for arm64
	MkdirAtNr        = unix.SYS_MKDIRAT           // MkdirAtNr defines the syscall ID for arm64
	UtimensAtNr      = unix.SYS_UTIMENSAT         // UtimensAtNr defines the syscall ID for arm64
	LinkAtNr         = unix.SYS_LINKAT            // LinkAtNr defines the syscall ID for arm64
	SymlinkAtNr      = unix.SYS_SYMLINKAT         // SymlinkAtNr defines the syscall ID for arm64
	FchmodNr         = unix.SYS_FCHMOD            // FchmodNr defines the syscall ID for arm64
	FchmodAtNr       = unix.SYS_FCHMODAT          // FchmodAtNr defines the syscall ID for arm64
	FchmodAt2Nr      = unix.SYS_FCHMODAT2         // FchmodAt2Nr defines the syscall ID for arm64
	FchownNr         = unix.SYS_FCHOWN            // FchownNr defines the syscall ID for arm64
	FchownAtNr       = unix.SYS_FCHOWNAT          // FchownAtNr defines the syscall ID for arm64
	InitModuleNr     = unix.SYS_INIT_MODULE       // InitModuleNr defines the syscall ID for arm64
	FInitModuleNr    = unix.SYS_FINIT_MODULE      // FInitModuleNr defines the syscall ID for arm64
	DeleteModuleNr   = unix.SYS_DELETE_MODULE     // DeleteModuleNr defines the syscall ID for arm64
	IoctlNr          = unix.SYS_IOCTL             // IoctlNr defines the syscall ID for arm64
	MountNr          = unix.SYS_MOUNT             // MountNr defines the syscall ID for arm64
	Umount2Nr        = unix.SYS_UMOUNT2           // Umount2Nr defines the syscall ID for arm64

	OpenNr      = -1  // OpenNr not available on arm64
	ForkNr      = -2  // ForkNr not available on arm64
	VforkNr     = -3  // VforkNr not available on arm64
	Dup2Nr      = -4  // Dup2Nr not available on arm64
	CreatNr     = -5  // CreatNr not available on arm64
	UnlinkNr    = -6  // UnlinkNr not available on arm64
	RmdirNr     = -7  // RmdirNr not available on arm64
	RenameNr    = -8  // RenameNr not available on arm64
	MkdirNr     = -9  // MkdirNr not available on arm64
	UtimeNr     = -10 // UtimeNr not available on arm64
	UtimesNr    = -11 // UtimesNr not available on arm64
	FutimesAtNr = -12 // FutimesAtNr not available on arm64
	LinkNr      = -13 // LinkNr not available on arm64
	SymlinkNr   = -14 // SymlinkNr not available on arm64
	ChmodNr     = -15 // ChmodNr not available on arm64
	ChownNr     = -16 // ChownNr not available on arm64
	LchownNr    = -17 // LchownNr not available on arm64
)

func (t *Tracer) argToRegValue(regs syscall.PtraceRegs, arg int) uint64 {
	if arg >= 0 && arg <= 5 {
		return regs.Regs[arg]
	}
	return 0
}

// ReadRet reads and returns the return value
func (t *Tracer) ReadRet(regs syscall.PtraceRegs) int64 {
	return int64(regs.Regs[0])
}

// GetSyscallNr returns the given syscall number
func GetSyscallNr(regs syscall.PtraceRegs) int {
	return int(regs.Regs[8])
}
