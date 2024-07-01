// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

import (
	"syscall"
)

const (
	// RPCCmd defines the ioctl CMD magic used by APM to register span TLS
	RPCCmd uint64 = 0xdeadc001
	// RegisterSpanTLSOp defines the span TLS register op code
	RegisterSpanTLSOp uint8 = 6
)

func registerERPCHandlers(handlers map[int]syscallHandler) []string {
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

func handleERPC(tracer *Tracer, process *Process, regs syscall.PtraceRegs) []byte {
	fd := tracer.ReadArgUint64(regs, 1)
	if fd != RPCCmd {
		return nil
	}

	pRequests, err := tracer.ReadArgData(process.Pid, regs, 2, 257)
	if err != nil || len(pRequests) == 0 {
		return nil
	}

	return pRequests
}
