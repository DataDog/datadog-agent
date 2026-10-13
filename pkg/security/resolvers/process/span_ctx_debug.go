// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TEMPORARY: span context resolution debugging for the TestOTelSpan flake. The
// functional tests log at warn, so everything here is a warning carrying the
// span-dbg marker the test greps for.

func spanDbgf(format string, args ...any) {
	seclog.Warnf("span-dbg: "+format, args...)
}

// DebugOTelProcessContextState describes what the resolver knows about pid's
// OTel process context resolution, for the functional tests to print when a
// resolution times out.
func (p *EBPFResolver) DebugOTelProcessContextState(pid uint32) string {
	p.otelProcCtxLock.Lock()
	queuedAt, pending := p.otelProcCtxPending[pid]
	pendingCount := len(p.otelProcCtxPending)
	p.otelProcCtxLock.Unlock()

	pendingFor := time.Duration(0)
	if pending {
		pendingFor = time.Since(queuedAt)
	}

	var tlsValue string
	if p.otelTLSMap == nil {
		tlsValue = "<no otel_tls map>"
	} else if value, err := p.otelTLSMap.LookupBytes(pid); err != nil {
		tlsValue = "lookup error: " + err.Error()
	} else if value == nil {
		tlsValue = "<absent>"
	} else {
		tlsValue = hex.EncodeToString(value)
	}

	return fmt.Sprintf("span tracking enabled=%t, has process entry=%t, pending=%t (for %s), pending pids=%d, queued=%d/%d, otel_tls[%d]=%s",
		p.config.SpanTrackingEnabled, p.hasEntry(pid), pending, pendingFor, pendingCount,
		len(p.otelProcCtxQueue), cap(p.otelProcCtxQueue), pid, tlsValue)
}

// DebugResolveOTelProcessContextNow resolves pid's OTel process context
// synchronously, bypassing the eBPF trigger and the queue, so a timed out test
// can tell a lost trigger from a broken resolution.
func (p *EBPFResolver) DebugResolveOTelProcessContextNow(pid uint32) {
	spanDbgf("pid %d: direct resolution requested by the test", pid)
	p.resolveAndUpdateOTelTLS(pid, new(otelTargetProcess))
}

// debugOTelMapsSummary re-reads pid's maps and keeps the lines a process
// context could live in.
func debugOTelMapsSummary(pid uint32) string {
	data, err := os.ReadFile(kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "maps"))
	if err != nil {
		return "maps re-read failed: " + err.Error()
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var kept []string
	for _, line := range lines {
		if strings.Contains(line, "OTEL") || strings.Contains(line, "memfd") || strings.Contains(line, "anon") {
			kept = append(kept, line)
		}
	}
	return fmt.Sprintf("%d maps lines, matching: %q", len(lines), kept)
}
