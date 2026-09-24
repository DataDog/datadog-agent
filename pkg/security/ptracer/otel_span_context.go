// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"fmt"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"golang.org/x/sys/unix"
)

// OTel Thread Local Context Record support (OTEP 4947), eBPF-less path.
//
// This wires the statically linked (local-exec TLS, module_id == 0) case:
// reading the record itself (thread pointer + a fixed TLS offset,
// otel_span_context_{amd64,unsupported}.go) is generic across processes, but
// resolving that offset needs one-time ELF analysis of the target's
// executable, triggered the same way the eBPF path triggers it: the OTEP
// 4719 readiness signal, prctl(PR_SET_VMA, PR_SET_VMA_ANON_NAME, ...,
// "OTEL_CTX"). See span_otel.h's handle_otel_process_ctx_naming/
// fill_span_context_otel for the eBPF-side equivalent of both halves.
//
// Known gaps, left for a real implementation:
//   - dynamically linked targets (module_id != 0, needs a live GOT/DTV read)
//     are not handled: they resolve as "not applicable" and are never
//     retried, exactly like a target with no OTel instrumentation at all.
//   - no attributes: only span_id/trace_id are attached.

// otelCtxVMAName is the OTEP 4719 readiness signal: the process names a VMA
// this to mean "otel_thread_ctx_v1 is registered, and OTel process context is
// at this address", mirroring OTEL_CTX_VMA_NAME in span_otel.h.
const otelCtxVMAName = "OTEL_CTX"

// otelStaticTLSEntry is Tracer.otelStaticTLS's cached resolution for one tgid.
// ok is false when the process was checked and does not export
// otel_thread_ctx_v1 through the local-exec TLS model this reader supports --
// cached too, so an uninstrumented (or dynamically linked) process is only
// checked once.
type otelStaticTLSEntry struct {
	offset int64
	ok     bool
}

// maybeRegisterOTelStaticTLS looks at a prctl(PR_SET_VMA, ...) call and, the
// first time it recognizes the OTel readiness signal for process's tgid,
// resolves and caches its static TLS offset.
func maybeRegisterOTelStaticTLS(tracer *Tracer, process *Process, regs syscall.PtraceRegs) {
	if tracer.ReadArgUint64(regs, 1) != unix.PR_SET_VMA_ANON_NAME {
		return
	}
	name, err := tracer.ReadArgString(process.Pid, regs, 4)
	if err != nil || name != otelCtxVMAName {
		return
	}
	tracer.registerOTelStaticTLS(process.Tgid)
}

// inheritOTelStaticTLS copies parentTgid's cached static TLS resolution (if
// any) to childTgid on a real fork, mirroring inherit_otel_tls() in
// span_otel.h: a forked child's address space is a copy-on-write duplicate
// of its parent's, so the same otel_thread_ctx_v1 export resolves to the
// exact same TP-relative offset -- no ELF work needs repeating, and nothing
// needs a live read, since this is a pure value copy.
//
// Only a genuine fork needs this: a CLONE_THREAD clone already gets the same
// Tgid as its parent (shareResources), so it resolves through the identical
// cache key without a copy.
func (t *Tracer) inheritOTelStaticTLS(parentTgid, childTgid int) {
	if entry, ok := t.otelStaticTLS[parentTgid]; ok {
		t.otelStaticTLS[childTgid] = entry
	}
}

// registerOTelStaticTLS resolves tgid's static TLS offset, once.
func (t *Tracer) registerOTelStaticTLS(tgid int) {
	if t.otelStaticTLS == nil {
		t.otelStaticTLS = make(map[int]otelStaticTLSEntry)
	}
	if _, done := t.otelStaticTLS[tgid]; done {
		return
	}

	offset, err := resolveOTelStaticTLSOffset(fmt.Sprintf("/proc/%d/exe", tgid))
	t.otelStaticTLS[tgid] = otelStaticTLSEntry{offset: offset, ok: err == nil}
}

// attachOTelStaticSpanContext fills msg.SpanContext from the current
// thread's (process.Pid, its tid) published OTel span, if any: process's
// tgid must have resolved a static TLS offset, and a valid record must be
// published right now. Called for every outgoing message, mirroring how
// fill_span_context_otel() runs on every eBPF-side event.
func (t *Tracer) attachOTelStaticSpanContext(process *Process, msg *ebpfless.SyscallMsg) {
	entry, done := t.otelStaticTLS[process.Tgid]
	if !done || !entry.ok {
		return
	}

	traceIDHi, traceIDLo, spanID, found, err := t.readOTelSpanContextStaticTLS(process.Pid, entry.offset)
	if err != nil || !found {
		return
	}

	msg.SpanContext = &ebpfless.SpanContext{
		SpanID:  spanID,
		TraceID: utils.TraceID{Hi: traceIDHi, Lo: traceIDLo},
	}
}
