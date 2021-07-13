// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/ebpf/manager"
)

const (
	rpcCmd = 0xdeadc010

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
	}
}

// Request generates an ioctl syscall with the required request
func (k *ERPC) Request(req *ERPCRequest) error {
	runtimeArch := probes.GetRuntimeArch()
	cmdAndOp := uintptr(rpcCmd) | uintptr(req.OP)

	var errno syscall.Errno
	if runtimeArch == "arm64" {
		if req.OP != DiscardInodeOp && req.OP != DiscardPidOp {
			return fmt.Errorf("eRPC op (%v) is not supported on this platform", req.OP)
		}

		var args [4]uintptr
		for i := 0; i < 4; i++ {
			args[i] = uintptr(model.ByteOrder.Uint64(req.Data[8*i : 8*(i+1)]))
		}

		_, _, errno = syscall.RawSyscall6(syscall.SYS_IOCTL, uintptr(k.fd), cmdAndOp, args[0], args[1], args[2], args[3])
	} else {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(k.fd), cmdAndOp, uintptr(unsafe.Pointer(&req.Data)))
	}

	runtime.KeepAlive(req)

	if errno != 0 && errno != syscall.ENOTTY {
		return errno
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
