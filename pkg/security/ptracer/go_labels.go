// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/golabels"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// tracerMemfdPrefix mirrors MEMFD_TRACER_PREFIX in
// pkg/security/ebpf/c/include/hooks/memfd.h, behind the prefix the ptracer
// names memfds with.
const tracerMemfdPrefix = "memfd:datadog-tracer-info-"

// isTracerMemfdSealing tells whether the fcntl seals a tracer-info memfd: the
// tracer published its metadata, which is when the eBPF probe resolves the
// pprof label offsets of a Go process.
func isTracerMemfdSealing(tracer *Tracer, process *Process, regs syscall.PtraceRegs) bool {
	if tracer.ReadArgInt32(regs, 1) != unix.F_ADD_SEALS {
		return false
	}

	// The fd maps to the name memfd_create reported, or, for a memfd the ptracer
	// did not see created, to the /proc/<pid>/fd link, which starts with a /.
	filename, err := process.GetFilenameFromFd(tracer.ReadArgInt32(regs, 0))
	return err == nil && strings.HasPrefix(strings.TrimPrefix(filename, "/"), tracerMemfdPrefix)
}

// resolveGoLabels resolves the pprof label offsets of the executable of
// process, at the exit of the fcntl that sealed its tracer-info memfd. Unlike
// the eBPF probe, it does not read the memfd to check that the tracer is
// dd-trace-go: a tracer of another language does not run in a Go executable,
// which ResolveLayout rejects. It runs on the trace loop, so every tracee waits
// on the ELF analysis.
func (ctx *CWSPtracerCtx) resolveGoLabels(process *Process) {
	// With no layout ever registered, goLabelsSpanContext reads nothing either.
	if ctx.opts.SpanContextDisabled {
		return
	}

	// The layout describes the executable, which only an exec replaces.
	if ctx.processCache.getGoLabels(process.Tgid) != nil {
		return
	}

	exePath := filepath.Join(otelProcRoot, strconv.Itoa(process.Tgid), "exe")
	layout, tlsErr, err := golabels.ResolveLayout(exePath)
	if err != nil {
		if !errors.Is(err, golabels.ErrNotGo) {
			logger.Debugf("unable to resolve the Go labels layout of %d: %v", process.Tgid, err)
		}
		return
	}
	if tlsErr != nil {
		logger.Debugf("Go labels TLS offset of %d: %v", process.Tgid, tlsErr)
	}

	ctx.processCache.setGoLabels(process.Tgid, &layout)
}

// goLabelsSpanContext reads the span context dd-trace-go set as pprof labels
// on the goroutine the thread of process runs. regs must be that thread's, in
// a ptrace-stop.
func (ctx *CWSPtracerCtx) goLabelsSpanContext(process *Process, regs *syscall.PtraceRegs) *ebpfless.SpanContext {
	layout := ctx.processCache.getGoLabels(process.Tgid)
	if layout == nil {
		return nil
	}

	// Only fetched when the runtime keeps g in TLS: on arm64 it costs a
	// PTRACE_GETREGSET.
	var tp uint64
	if layout.TLSOffset() != 0 {
		var err error
		if tp, err = threadPointer(process.Pid, regs); err != nil {
			logger.Debugf("unable to read the thread pointer of %d: %v", process.Pid, err)
		}
	}

	labels, status := layout.ReadLabels(tracerMemory{pid: process.Pid}, tp, goGRegister(regs))
	if status != golabels.ReadOK {
		return nil
	}

	spanID, traceID := golabels.SpanContext(labels)
	if spanID == 0 && traceID == (utils.TraceID{}) {
		return nil
	}

	return &ebpfless.SpanContext{
		SpanID:  spanID,
		TraceID: traceID,
	}
}

// spanContext reads the span context of the thread of process, from whichever
// reader the process registered. regs must be that thread's, in a ptrace-stop.
func (ctx *CWSPtracerCtx) spanContext(process *Process, regs *syscall.PtraceRegs) *ebpfless.SpanContext {
	if spanCtx := ctx.otelSpanContext(process, regs); spanCtx != nil {
		return spanCtx
	}
	return ctx.goLabelsSpanContext(process, regs)
}
