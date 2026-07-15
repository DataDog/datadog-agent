// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"

	iouring "github.com/iceber/iouring-go"
	iouring_syscall "github.com/iceber/iouring-go/syscall"
)

func ioUringResult(result iouring.Result) (int, error) {
	req, ok := result.(iouring.Request)
	if !ok {
		return 0, fmt.Errorf("unexpected io_uring result type %T", result)
	}
	return req.GetRes()
}

// The iouring-go fork only wraps a subset of opcodes. The helpers below build raw
// submission queue entries for opcodes that have no prep helper, mirroring the field
// layout the kernel expects in its *_prep handlers.

func ioUringPrepSocket(domain, typ, protocol int) iouring.PrepRequest {
	return func(sqe iouring_syscall.SubmissionQueueEntry, _ *iouring.UserData) {
		sqe.PrepOperation(iouring_syscall.IORING_OP_SOCKET, int32(domain), 0, uint32(protocol), uint64(typ))
	}
}

func ioUringPrepFtruncate(fd int, length int64) iouring.PrepRequest {
	return func(sqe iouring_syscall.SubmissionQueueEntry, _ *iouring.UserData) {
		sqe.PrepOperation(iouring_syscall.IORING_OP_FTRUNCATE, int32(fd), 0, 0, uint64(length))
	}
}

func ioUringPrepSplice(fdIn, fdOut, length int) iouring.PrepRequest {
	return func(sqe iouring_syscall.SubmissionQueueEntry, _ *iouring.UserData) {
		sqe.PrepOperation(iouring_syscall.IORING_OP_SPLICE, int32(fdOut), ^uint64(0), uint32(length), ^uint64(0))
		sqe.SetSpliceFdIn(int32(fdIn))
	}
}
