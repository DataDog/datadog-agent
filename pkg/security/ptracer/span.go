// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

import (
	"encoding/binary"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

const (
	RPC_CMD              uint64 = 0xdeadc001
	REGISTER_SPAN_TLS_OP uint8  = 6
)

type spanTLS struct {
	format     uint64
	maxThreads uint64
	base       uintptr
}

func registerSpanHandlers(handlers map[int]syscallHandler) []string {
	fimHandlers := []syscallHandler{
		{
			IDs:        []syscallID{{ID: IoctlNr, Name: "ioctl"}},
			Func:       nil,
			ShouldSend: nil,
			RetFunc:    nil,
		},
	}
	syscallList := []string{}
	for _, h := range fimHandlers {
		for _, id := range h.IDs {
			if id.ID >= 0 { // insert only available syscalls
				handlers[id.ID] = h
				syscallList = append(syscallList, id.Name)
			}
		}
	}
	return syscallList
}

func handleIoctl(tracer *Tracer, process *Process, regs syscall.PtraceRegs) *spanTLS {
	fd := tracer.ReadArgUint64(regs, 1)
	if fd != RPC_CMD {
		return nil
	}

	pRequests, err := tracer.ReadArgData(process.Pid, regs, 2, 257)
	if err != nil || pRequests[0] != REGISTER_SPAN_TLS_OP {
		return nil
	}

	return &spanTLS{
		format:     binary.NativeEndian.Uint64(pRequests[1:9]),
		maxThreads: binary.NativeEndian.Uint64(pRequests[9:17]),
		base:       uintptr(binary.NativeEndian.Uint64(pRequests[17:25])),
	}
}

func fillSpanContext(tracer *Tracer, pid int, tid int, span *spanTLS) *ebpfless.SpanContext {
	if span == nil {
		return nil
	}
	offset := uint64((tid % int(span.maxThreads)) * 2 * 8)

	pSpan, err := tracer.readData(pid, uint64(span.base)+offset, 16 /*sizeof uint64 x2*/)
	if err != nil {
		return nil
	}

	return &ebpfless.SpanContext{
		SpanID:  binary.NativeEndian.Uint64(pSpan[0:8]),
		TraceID: binary.NativeEndian.Uint64(pSpan[8:16]),
	}
}
