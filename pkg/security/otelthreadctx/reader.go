// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelthreadctx

import (
	"encoding/binary"

	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// Mirrors pkg/security/ebpf/c/include/helpers/span_otel.h hop for hop: change
// both together.

// ReadStatus mirrors SPAN_CTX_EVENT_* in pkg/security/ebpf/c/include/constants/custom.h.
type ReadStatus int

// ReadStatus values
const (
	ReadOK ReadStatus = iota
	ReadNone
	ReadNoThreadPointer
	ReadFault
	ReadTorn
	ReadAttrsFault // the ids are still returned, without attributes
)

// Record is an OTEP 4947 Thread Local Context Record.
type Record struct {
	TraceID utils.TraceID
	SpanID  uint64
	Attrs   []byte // raw attrs_data, see ParseAttributes
}

const (
	// trace_id[16] span_id[8] valid[1] reserved[1] attrs_data_size[2]
	otelRecordHeaderSize      = 28
	otelRecordValidOffset     = 24
	otelRecordAttrsSizeOffset = 26
	// OTEL_ATTRS_MAX_SIZE: the ptracer could read more, but both probes must
	// report the same attributes.
	otelAttrsMaxSize = 256
)

// ReadRecord reads the record the thread whose thread pointer is threadPointer
// publishes, out of mem, the memory of its process.
func (res Layout) ReadRecord(mem otelprocessctx.Memory, threadPointer uint64) (Record, ReadStatus) {
	// Go publishes through pprof labels, and Node.js is not read in user space
	// yet.
	if res.runtimeLang != otelRuntimeNative {
		return Record{}, ReadNone
	}
	if threadPointer == 0 {
		return Record{}, ReadNoThreadPointer
	}

	varAddr, status := res.tlsVarAddr(mem, threadPointer)
	if status != ReadOK {
		return Record{}, status
	}

	recordPtr, err := readWord(mem, varAddr)
	if err != nil {
		return Record{}, ReadFault
	}
	if recordPtr == 0 {
		return Record{}, ReadNone
	}
	return readRecordAt(mem, recordPtr)
}

// tlsVarAddr mirrors otel_tls_var_addr. A failed DTV read is ReadNone, not
// ReadFault: it is a thread that never touched the module's TLS block.
func (res Layout) tlsVarAddr(mem otelprocessctx.Memory, threadPointer uint64) (uint64, ReadStatus) {
	tlsBlock := threadPointer

	if res.moduleID != 0 {
		dtv, err := readWord(mem, threadPointer+uint64(res.dtvInfo.offset))
		if err != nil {
			return 0, ReadNone
		}

		// DTV: [generation, module1_block, module2_block, ...]
		tlsBlock, err = readWord(mem, dtv+uint64(res.moduleID)*uint64(res.dtvInfo.multiplier))
		if err != nil {
			return 0, ReadNone
		}
	}

	addr := tlsBlock + uint64(res.tlsOffset)
	if addr == 0 {
		return 0, ReadNone
	}
	return addr, ReadOK
}

// readRecordAt mirrors otel_fill_from_record.
func readRecordAt(mem otelprocessctx.Memory, recordPtr uint64) (Record, ReadStatus) {
	// The writer clears valid while it updates the record: checking it on both
	// sides of the copy rejects a torn read.
	var validBefore [1]byte
	if err := mem.Read(recordPtr+otelRecordValidOffset, validBefore[:]); err != nil {
		return Record{}, ReadFault
	}
	if validBefore[0] != 1 {
		return Record{}, ReadNone
	}

	var header [otelRecordHeaderSize]byte
	if err := mem.Read(recordPtr, header[:]); err != nil {
		return Record{}, ReadFault
	}

	var validAfter [1]byte
	if err := mem.Read(recordPtr+otelRecordValidOffset, validAfter[:]); err != nil {
		return Record{}, ReadFault
	}
	if header[otelRecordValidOffset] != 1 || validAfter[0] != 1 {
		return Record{}, ReadTorn
	}

	// The W3C trace id is big-endian: bytes[0..7] are its high 64 bits.
	record := Record{
		TraceID: utils.TraceID{
			Hi: binary.BigEndian.Uint64(header[0:8]),
			Lo: binary.BigEndian.Uint64(header[8:16]),
		},
		SpanID: binary.BigEndian.Uint64(header[16:24]),
	}

	attrsSize := min(int(binary.NativeEndian.Uint16(header[otelRecordAttrsSizeOffset:])), otelAttrsMaxSize)
	if attrsSize > 0 {
		attrs := make([]byte, attrsSize)
		if err := mem.Read(recordPtr+otelRecordHeaderSize, attrs); err != nil {
			return record, ReadAttrsFault
		}
		record.Attrs = attrs
	}

	return record, ReadOK
}

func readWord(mem otelprocessctx.Memory, addr uint64) (uint64, error) {
	var buf [8]byte
	if err := mem.Read(addr, buf[:]); err != nil {
		return 0, err
	}
	return binary.NativeEndian.Uint64(buf[:]), nil
}
