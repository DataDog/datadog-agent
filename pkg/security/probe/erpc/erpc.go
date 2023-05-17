// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux

package erpc

import (
	"errors"
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
	RegisterSpanTLSOP
	// ExpireInodeDiscarderOp is used to expire an inode discarder
	ExpireInodeDiscarderOp
	// ExpirePidDiscarderOp is used to expire a pid discarder
	ExpirePidDiscarderOp
	// BumpDiscardersRevision is used to bump the discarders revision
	BumpDiscardersRevision
	// GetRingbufUsage is used to retrieve the ring buffer usage
	GetRingbufUsage
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
	if req.OP == 0 {
		return errors.New("no op provided")
	}

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
