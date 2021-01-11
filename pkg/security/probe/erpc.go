// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/DataDog/ebpf/manager"
)

const (
	rpcCmd = 0xdeadc001

	// ERPCMaxDataSize maximum size of data of a request
	ERPCMaxDataSize = 256
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

// GetConstants returns the ebpf constants
func (k *ERPC) GetConstants() []manager.ConstantEditor {
	return []manager.ConstantEditor{
		{
			Name:  "erpc_fd",
			Value: uint64(k.fd),
		},
		{
			Name:  "erpc_pid",
			Value: uint64(os.Getpid()),
		},
	}
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
