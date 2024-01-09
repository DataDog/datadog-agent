// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ProgObjInfo retrieves information about a BPF Fd.
func ProgObjInfo(fd uint32, info *ProgInfo) error {
	err := ObjGetInfoByFd(&ObjGetInfoByFdAttr{
		BpfFd:   fd,
		InfoLen: uint32(unsafe.Sizeof(*info)),
		Info:    NewPointer(unsafe.Pointer(info)),
	})
	return err
}

// ObjGetInfoByFdAttr is the attributes for the BPF_OBJ_GET_INFO_BY_FD mode of the bpf syscall
type ObjGetInfoByFdAttr struct {
	BpfFd   uint32
	InfoLen uint32
	Info    Pointer
}

// ObjGetInfoByFd implements the BPF_OBJ_GET_INFO_BY_FD mode of the bpf syscall
func ObjGetInfoByFd(attr *ObjGetInfoByFdAttr) error {
	_, _, errNo := unix.Syscall(unix.SYS_BPF, uintptr(unix.BPF_OBJ_GET_INFO_BY_FD), uintptr(unsafe.Pointer(attr)), unsafe.Sizeof(*attr))
	if errNo != 0 {
		return errNo
	}
	runtime.KeepAlive(attr)
	return nil
}

// ProgGetFdByIDAttr corresponds to the subset of `bpf_prog_attr` needed for BPF_PROG_GET_FD_BY_ID
type ProgGetFdByIDAttr struct {
	ID uint32
}

// ProgGetFdByID opens a file descriptor for the eBPF program corresponding to ID in attr
func ProgGetFdByID(attr *ProgGetFdByIDAttr) (uint32, error) {
	fd, _, errNo := unix.Syscall(unix.SYS_BPF, uintptr(unix.BPF_PROG_GET_FD_BY_ID), uintptr(unsafe.Pointer(attr)), unsafe.Sizeof(*attr))
	if errNo != 0 {
		return 0, errNo
	}
	runtime.KeepAlive(attr)
	return uint32(fd), nil
}

// ProgInfo corresponds to kernel C type `bpf_prog_info`
type ProgInfo struct {
	Type                 uint32
	ID                   uint32
	Tag                  [8]uint8
	JitedProgLen         uint32
	XlatedProgLen        uint32
	JitedProgInsns       uint64
	XlatedProgInsns      Pointer
	LoadTime             uint64
	CreatedByUID         uint32
	NrMapIds             uint32
	MapIds               Pointer
	Name                 ObjName
	Ifindex              uint32
	_                    [4]byte /* unsupported bitfield */
	NetnsDev             uint64
	NetnsIno             uint64
	NrJitedKsyms         uint32
	NrJitedFuncLens      uint32
	JitedKsyms           uint64
	JitedFuncLens        uint64
	BtfID                BTFID
	FuncInfoRecSize      uint32
	FuncInfo             uint64
	NrFuncInfo           uint32
	NrLineInfo           uint32
	LineInfo             uint64
	JitedLineInfo        uint64
	NrJitedLineInfo      uint32
	LineInfoRecSize      uint32
	JitedLineInfoRecSize uint32
	NrProgTags           uint32
	ProgTags             uint64
	RunTimeNs            uint64
	RunCnt               uint64
	RecursionMisses      uint64
	VerifiedInsns        uint32
	_                    [4]byte
}

// Pointer wraps an unsafe.Pointer to be 64bit to
// conform to the syscall specification.
type Pointer struct {
	ptr unsafe.Pointer
}

// NewPointer creates a 64-bit pointer from an unsafe Pointer.
func NewPointer(ptr unsafe.Pointer) Pointer {
	return Pointer{ptr: ptr}
}

// ObjName is a null-terminated string made up of
// 'A-Za-z0-9_' characters.
type ObjName [unix.BPF_OBJ_NAME_LEN]byte

// BTFID uniquely identifies a BTF blob loaded into the kernel.
type BTFID uint32
