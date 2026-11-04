// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package awk

import (
	"context"
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

const programReadWaitMilliseconds = 25

type pollingProgramFile struct {
	ctx  context.Context
	file programFile
	conn syscall.RawConn
}

func (r *pollingProgramFile) Read(p []byte) (int, error) {
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		ready := false
		var readyErr error
		err := r.conn.Control(func(fd uintptr) {
			ready, readyErr = programFileReadReady(fd)
		})
		if err != nil {
			return 0, err
		}
		if readyErr != nil {
			if ctxErr := r.ctx.Err(); ctxErr != nil {
				return 0, ctxErr
			}
			if readyErr == unix.EINTR {
				continue
			}
			return 0, readyErr
		}
		if !ready {
			continue
		}
		n, err := r.file.Read(p)
		if n == 0 && err == nil {
			return 0, io.EOF
		}
		return n, err
	}
}

func readProgramFileFallback(ctx context.Context, file programFile, total *int) (string, bool, error) {
	type syscallProgramFile interface {
		programFile
		SyscallConn() (syscall.RawConn, error)
	}
	syscallFile, ok := file.(syscallProgramFile)
	if !ok {
		return "", false, nil
	}
	conn, err := syscallFile.SyscallConn()
	if err != nil {
		return "", true, err
	}
	flags := 0
	var controlErr error
	err = conn.Control(func(fd uintptr) {
		flags, controlErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
	})
	if err != nil {
		return "", true, err
	}
	if controlErr != nil {
		return "", true, controlErr
	}
	if flags&unix.O_NONBLOCK != 0 {
		return "", false, nil
	}
	// Reading an awk program requires exclusive logical ownership of stdin.
	// Bounded readiness waits keep cancellation responsive without changing
	// caller-owned fd flags.
	text, err := readProgram(ctx, &pollingProgramFile{ctx: ctx, file: file, conn: conn}, total)
	return text, true, err
}
