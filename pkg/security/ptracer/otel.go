// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
	"github.com/DataDog/datadog-agent/pkg/security/otelthreadctx"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

// Not HostProc: the ptracer shares the pid namespace of its tracees.
const otelProcRoot = "/proc"

// otelState is the OTel thread context registration of a process.
type otelState struct {
	layout        otelthreadctx.Layout
	attributeKeys []string
}

// otelRegistry stores layouts in the process cache.
type otelRegistry struct {
	cache *ProcessCache
}

// Registered implements otelthreadctx.Registry.
func (r otelRegistry) Registered(pid uint32) bool {
	return r.cache.getOTel(int(pid)) != nil
}

// Register implements otelthreadctx.Registry.
func (r otelRegistry) Register(pid uint32, layout otelthreadctx.Layout) error {
	r.cache.setOTel(int(pid), &otelState{layout: layout})
	return nil
}

// isOTelProcessCtxNaming mirrors is_otel_process_ctx_naming in
// pkg/security/ebpf/c/include/helpers/span_otel.h.
func isOTelProcessCtxNaming(tracer *Tracer, process *Process, regs syscall.PtraceRegs) bool {
	if tracer.ReadArgInt32(regs, 0) != unix.PR_SET_VMA || tracer.ReadArgUint64(regs, 1) != unix.PR_SET_VMA_ANON_NAME {
		return false
	}

	name, err := tracer.ReadArgString(process.Pid, regs, 4)
	return err == nil && name == otelprocessctx.MappingName
}

// resolveOTelProcessContext runs at the prctl exit: /proc/<pid>/maps only shows
// the mapping name once the prctl returned. Its return value is ignored, as
// OTEP 4719 does not require the naming to succeed. It runs on the trace loop,
// so every tracee waits on the ELF analysis.
func (ctx *CWSPtracerCtx) resolveOTelProcessContext(process *Process) {
	// With no layout ever registered, otelSpanContext reads nothing either.
	if ctx.opts.SpanContextDisabled {
		return
	}

	ctx.otelTarget.Reset(uint32(process.Tgid))

	res, err := ctx.otelTarget.Resolve(otelRegistry{cache: ctx.processCache})
	if err != nil {
		logger.Debugf("unable to resolve the OTel process context of %d: %v", process.Tgid, err)
	}

	if state := ctx.processCache.getOTel(process.Tgid); err == nil && state != nil {
		state.attributeKeys = res.AttributeKeys
	}
}

// otelSpanContext reads the record of the thread of process. regs must be that
// thread's, in a ptrace-stop.
func (ctx *CWSPtracerCtx) otelSpanContext(process *Process, regs *syscall.PtraceRegs) *ebpfless.SpanContext {
	state := ctx.processCache.getOTel(process.Tgid)
	if state == nil {
		return nil
	}

	tp, err := threadPointer(process.Pid, regs)
	if err != nil {
		logger.Debugf("unable to read the thread pointer of %d: %v", process.Pid, err)
		return nil
	}

	record, status := state.layout.ReadRecord(tracerMemory{pid: process.Pid}, tp)
	switch status {
	case otelthreadctx.ReadOK, otelthreadctx.ReadAttrsFault:
	default:
		return nil
	}

	return &ebpfless.SpanContext{
		SpanID:     record.SpanID,
		TraceID:    record.TraceID,
		Attributes: otelthreadctx.NamedAttributes(state.attributeKeys, otelthreadctx.ParseAttributes(record.Attrs)),
	}
}

var errShortRead = errors.New("short read")

// tracerMemory reads a tracee with process_vm_readv.
type tracerMemory struct {
	pid int
}

// Read implements otelprocessctx.Memory.
func (m tracerMemory) Read(addr uint64, buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	n, err := processVMReadv(m.pid, uintptr(addr), buf)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return fmt.Errorf("%w: %d bytes out of %d at %#x", errShortRead, n, len(buf), addr)
	}
	return nil
}
