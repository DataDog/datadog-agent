// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package otelprocessctx reads the OpenTelemetry process context a traced process
// publishes, as specified by OTEP 4719.
//
// The publisher maps a page holding a fixed 32-byte header, names it OTEL_CTX so a
// reader can find it in /proc/<pid>/maps, and points the header at a protobuf payload
// living elsewhere in its address space. Header and payload are therefore both read
// out of the target process; the mapping's own contents are only the header.
package otelprocessctx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"

	processcontextpb "go.opentelemetry.io/proto/slim/otlp/processcontext/v1development"
	"google.golang.org/protobuf/proto"
)

const (
	// MappingName is the name the publisher gives the mapping holding the header.
	MappingName = "OTEL_CTX"

	// headerSize is the size of the OTEP 4719 mapping header.
	headerSize = 32
	// headerVersion is the only process context version this reader understands.
	headerVersion = 2
	// maxPayloadSize bounds what is copied out of the target process. The payload is a
	// handful of resource attributes; anything larger is a misread header.
	maxPayloadSize = 1 << 20

	// readAttempts is how many times a torn read is retried. The publisher only writes
	// when the process context changes, which happens at most a few times per process,
	// so losing two races in a row means something else is wrong.
	readAttempts = 3
)

// unpublished is the monotonic_published_at_ns sentinel meaning the context is either
// not published yet or in the middle of an update.
const unpublished = 0

var signature = [8]byte{'O', 'T', 'E', 'L', '_', 'C', 'T', 'X'}

// header mirrors the OTEP 4719 mapping header.
type header struct {
	signature              [8]byte
	version                uint32
	payloadSize            uint32
	monotonicPublishedAtNs uint64
	payloadPtr             uint64
}

func parseHeader(buf []byte) (header, error) {
	var h header
	if len(buf) < headerSize {
		return h, fmt.Errorf("short header: %d bytes", len(buf))
	}
	copy(h.signature[:], buf[0:8])
	if h.signature != signature {
		return h, fmt.Errorf("bad signature %q", buf[0:8])
	}
	h.version = binary.NativeEndian.Uint32(buf[8:12])
	if h.version != headerVersion {
		return h, fmt.Errorf("unsupported process context version %d", h.version)
	}
	h.payloadSize = binary.NativeEndian.Uint32(buf[12:16])
	h.monotonicPublishedAtNs = binary.NativeEndian.Uint64(buf[16:24])
	h.payloadPtr = binary.NativeEndian.Uint64(buf[24:32])
	return h, nil
}

// IsMappingName reports whether a /proc/<pid>/maps pathname designates the process
// context mapping. The publisher prefers a memfd and falls back to an anonymous
// mapping named through prctl(PR_SET_VMA_ANON_NAME), hence the three spellings.
func IsMappingName(pathname string) bool {
	pathname = strings.TrimSuffix(pathname, " (deleted)")
	switch pathname {
	case "/memfd:" + MappingName, "[anon_shmem:" + MappingName + "]", "[anon:" + MappingName + "]":
		return true
	}
	return false
}

// Read returns the process context whose header sits at headerAddr in mem.
//
// The publication protocol is a seqlock: the publisher zeroes the timestamp, writes the
// payload location, then stores a strictly increasing timestamp. A payload is only
// accepted if the timestamp is non-zero and unchanged across the copy.
func Read(mem procfs.Mem, headerAddr uint64) (ProcessContext, error) {
	var lastErr error
	for range readAttempts {
		ctx, torn, err := readOnce(mem, headerAddr)
		if err == nil {
			return ctx, nil
		}
		if !torn {
			return nil, err
		}
		lastErr = err
	}

	return nil, lastErr
}

func readOnce(mem procfs.Mem, headerAddr uint64) (ProcessContext, bool, error) {
	buf := make([]byte, headerSize)
	if err := mem.Read(headerAddr, buf); err != nil {
		return nil, false, fmt.Errorf("cannot read the process context header: %w", err)
	}

	before, err := parseHeader(buf)
	if err != nil {
		return nil, false, err
	}
	if before.monotonicPublishedAtNs == unpublished {
		return nil, false, errors.New("process context is unpublished or being updated")
	}
	if before.payloadSize == 0 || before.payloadSize > maxPayloadSize {
		return nil, false, fmt.Errorf("implausible process context payload size %d", before.payloadSize)
	}
	if before.payloadPtr == 0 {
		return nil, false, errors.New("null process context payload pointer")
	}

	payload := make([]byte, before.payloadSize)
	if err := mem.Read(before.payloadPtr, payload); err != nil {
		return nil, false, fmt.Errorf("cannot read the process context payload: %w", err)
	}

	if err := mem.Read(headerAddr, buf); err != nil {
		return nil, false, fmt.Errorf("cannot re-read the process context header: %w", err)
	}
	after, err := parseHeader(buf)
	if err != nil {
		return nil, false, err
	}
	if after.monotonicPublishedAtNs != before.monotonicPublishedAtNs {
		return nil, true, errors.New("process context changed while being read")
	}

	var pb processcontextpb.ProcessContext
	if err := proto.Unmarshal(payload, &pb); err != nil {
		return nil, false, err
	}

	return &pb, false, nil
}
