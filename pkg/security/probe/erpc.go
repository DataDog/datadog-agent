// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"syscall"
	"unsafe"
)

const (
	rpcCmd = 0xdeadc001

	// ERPCMaxDataSize maximum size of data of a request
	ERPCMaxDataSize = 256
)

const (
	// DiscardInodeOp discards an inode
	DiscardInodeOp = iota + 1
	// DiscardPidOp discards a pid
	DiscardPidOp
	// ResolveSegmentOp resolves the requested segment
	ResolveSegmentOp
	// ResolvePathOp resolves the requested path
	ResolvePathOp
	// ResolveParentOp resolves the parent of the provide path key
	ResolveParentOp
	// RegisterSpanTLSOP is used for span TLS registration
	RegisterSpanTLSOP //nolint:deadcode,unused
	// ExpireInodeDiscarderOp is used to expire an inode discarder
	ExpireInodeDiscarderOp
)

// ERPC defines a krpc object
type ERPC struct {
	fd int
}

// ERPCRequest defines a EPRC request
type ERPCRequest struct {
	OP   uint8
	Data [ERPCMaxDataSize]byte
}

// Request generates an ioctl syscall with the required request
func (k *ERPC) Request(req *ERPCRequest) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(k.fd), rpcCmd, uintptr(unsafe.Pointer(req))); errno != 0 {
		if errno != syscall.ENOTTY {
			return errno
		}
	}

	return nil
}

// NewERPC returns a new ERPC object
func NewERPC() (*ERPC, error) {
	fd, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return nil, err
	}

	return &ERPC{
		fd: fd,
	}, nil
}
