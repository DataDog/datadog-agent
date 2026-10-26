// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package golabels resolves and reads the pprof labels dd-trace-go sets on the
// goroutine running a span. It is shared by the eBPF probe, which resolves the
// layout here and reads the labels in eBPF, and by the ptracer, which does both
// here. It stays free of agent dependencies so that the ptracer, which ships in
// cws-instrumentation, can link it.
package golabels

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"

	"github.com/go-delve/delve/pkg/goversion"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process/gooffsets"
)

// Sentinels ResolveLayout wraps its errors with, for the eBPF resolver's span
// context stats to classify them.
var (
	// ErrNotGo marks an executable that is not a Go binary.
	ErrNotGo = errors.New("not a Go binary")
	// ErrUnsupported marks a Go binary this code recognizes but cannot handle.
	ErrUnsupported = errors.New("unsupported")
	// ErrMalformed marks a Go binary whose metadata could not be read.
	ErrMalformed = errors.New("malformed")
)

var (
	// errDecodeSymbol is returned when the TLS offset could not be recovered from
	// the instruction stream of the symbol we decode.
	errDecodeSymbol = errors.New("failed to decode symbol")
	// errRuntimeIsCgoUnavailable is returned when runtime.iscgo could not be read
	// out of the binary, so we cannot tell whether the runtime keeps g in TLS.
	errRuntimeIsCgoUnavailable = errors.New("runtime.iscgo value unavailable")
)

// SerializedSize is the serialized size of go_labels_offsets_t:
// 6 * u32 + 1 * s32 = 28 bytes.
const SerializedSize = 28

// Layout is what reading the pprof labels of a Go executable takes: the
// runtime struct offsets of its Go version, and where its runtime keeps g.
type Layout struct {
	offsets gooffsets.GoRuntimeOffsets
	// tlsOffset is the offset of g from the thread pointer. 0 means the runtime
	// does not keep g in TLS, see extractTLSGOffset.
	tlsOffset int32
}

// TLSOffset returns the offset of g from the thread pointer, 0 when the runtime
// keeps g in a register only.
func (l Layout) TLSOffset() int32 {
	return l.tlsOffset
}

// ResolveLayout resolves the layout of the Go executable at exePath.
//
// tlsErr is a non-fatal error: the TLS offset could not be decoded, and the
// layout falls back to the best value extractTLSGOffset could come up with (the
// conventional offset on amd64, 0 on arm64). A 0 offset means "g is not in
// TLS", and the readers fall back to the g register where the ABI has one.
func ResolveLayout(exePath string) (layout Layout, tlsErr error, err error) {
	elfFile, err := pfelf.Open(exePath)
	if err != nil {
		return Layout{}, nil, fmt.Errorf("failed to open ELF: %w", err)
	}
	defer elfFile.Close()

	// Detect the Go version from the binary. This reads .go.buildinfo, which
	// survives `-ldflags=-s -w`.
	goVersion, err := elfFile.GoVersion()
	if err != nil {
		return Layout{}, nil, fmt.Errorf("%w: failed to read Go build info: %w", ErrMalformed, err)
	}
	if goVersion == "" {
		return Layout{}, nil, ErrNotGo
	}

	parsedGoVersion, ok := goversion.Parse(goVersion)
	if !ok {
		return Layout{}, nil, fmt.Errorf("%w: failed to parse Go version %s", ErrUnsupported, goVersion)
	}
	// Compare by major.minor only: the runtime layout only changes between minors.
	minorGoVersion := goversion.GoVersion{Major: parsedGoVersion.Major, Minor: parsedGoVersion.Minor}

	// A version newer than the ones the offsets were generated from must be rejected.
	if !minorGoVersion.AfterOrEqual(gooffsets.MinGoVersion) || minorGoVersion.AfterOrEqual(gooffsets.MaxGoVersion) {
		return Layout{}, nil, fmt.Errorf("%w: Go version %s (need >= %s and < %s)", ErrUnsupported, goVersion, gooffsets.MinGoVersion.String(), gooffsets.MaxGoVersion.String())
	}

	// Get struct offsets from the generated version table.
	layout.offsets, err = gooffsets.GetGoRuntimeOffsets(minorGoVersion, runtime.GOARCH)
	if err != nil {
		return Layout{}, nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}

	// Get the TLS G offset by decoding the runtime's own g-load sequence.
	layout.tlsOffset, err = extractTLSGOffset(elfFile)
	switch {
	case err == nil:
	case errors.Is(err, errDecodeSymbol), errors.Is(err, errRuntimeIsCgoUnavailable):
		tlsErr = err
	default:
		return Layout{}, nil, fmt.Errorf("%w: failed to extract TLS G offset: %w", ErrMalformed, err)
	}

	return layout, tlsErr, nil
}

// Serialize serializes the layout as the go_labels_offsets_t value of the
// go_labels_procs BPF map.
func (l Layout) Serialize() []byte {
	buf := make([]byte, SerializedSize)
	binary.NativeEndian.PutUint32(buf[0:4], l.offsets.MOffset)
	binary.NativeEndian.PutUint32(buf[4:8], l.offsets.Curg)
	binary.NativeEndian.PutUint32(buf[8:12], l.offsets.Labels)
	binary.NativeEndian.PutUint32(buf[12:16], l.offsets.HmapCount)
	binary.NativeEndian.PutUint32(buf[16:20], l.offsets.HmapLog2BucketCount)
	binary.NativeEndian.PutUint32(buf[20:24], l.offsets.HmapBuckets)
	binary.NativeEndian.PutUint32(buf[24:28], uint32(l.tlsOffset))
	return buf
}
