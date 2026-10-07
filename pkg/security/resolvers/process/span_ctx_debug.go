// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/binary"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// TEMPORARY: tracing of the OTel process context resolution, to find out why
// the Node.js span tests sometimes see an event with no span context at all.
// Logged at warn because that is the level the functional tests run at.

// spanCtxDebugf logs one step of the OTel process context resolution.
func spanCtxDebugf(format string, args ...any) {
	seclog.Warnf("[span-ctx-debug] "+format, args...)
}

// describeOTelTLSValue decodes a serialized struct otel_tls_t.
func describeOTelTLSValue(value []byte) string {
	if len(value) < otelTLSValueSize {
		return fmt.Sprintf("short value (%d bytes)", len(value))
	}
	return fmt.Sprintf("runtime=%d module_id=%d tls_offset=%d dtv_offset=%d dtv_multiplier=%d v8={tagged_size=%d js_map_table_offset=%#x ohm_header_size=%#x}",
		binary.NativeEndian.Uint32(value[0:4]),
		binary.NativeEndian.Uint32(value[4:8]),
		int64(binary.NativeEndian.Uint64(value[8:16])),
		int64(binary.NativeEndian.Uint64(value[16:24])),
		binary.NativeEndian.Uint32(value[24:28]),
		binary.NativeEndian.Uint16(value[32:34]),
		binary.NativeEndian.Uint16(value[34:36]),
		binary.NativeEndian.Uint16(value[36:38]),
	)
}

// DebugOTelTLSEntry describes what the otel_tls map holds for pid right now,
// for a test to log when an event carries no span context.
func (p *EBPFResolver) DebugOTelTLSEntry(pid uint32) string {
	if p.otelTLSMap == nil {
		return "otel_tls map not loaded"
	}

	p.otelProcCtxLock.Lock()
	_, pending := p.otelProcCtxPending[pid]
	p.otelProcCtxLock.Unlock()

	value, err := p.otelTLSMap.LookupBytes(pid)
	switch {
	case err != nil:
		return fmt.Sprintf("lookup error: %v (pending=%t, has_entry=%t)", err, pending, p.hasEntry(pid))
	case value == nil:
		return fmt.Sprintf("no otel_tls entry (pending=%t, has_entry=%t)", pending, p.hasEntry(pid))
	default:
		return fmt.Sprintf("%s (pending=%t, has_entry=%t)", describeOTelTLSValue(value), pending, p.hasEntry(pid))
	}
}
