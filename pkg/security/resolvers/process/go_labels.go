// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strconv"

	"github.com/go-delve/delve/pkg/goversion"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process/gooffsets"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// goLabelsOffsetsValueSize is the serialized size of go_labels_offsets_t:
// 6 * u32 + 1 * s32 = 28 bytes.
const goLabelsOffsetsValueSize = 28

var (
	// errDecodeSymbol is returned when the TLS offset could not be recovered from
	// the instruction stream of the symbol we decode.
	errDecodeSymbol = errors.New("failed to decode symbol")
	// errRuntimeIsCgoUnavailable is returned when runtime.iscgo could not be read
	// out of the binary, so we cannot tell whether the runtime keeps g in TLS.
	errRuntimeIsCgoUnavailable = errors.New("runtime.iscgo value unavailable")
)

// resolveGoLabels discovers the Go runtime offsets for pprof label reading
// and pushes them to the go_labels_procs BPF map.
func (p *EBPFResolver) resolveGoLabels(pid uint32) error {
	if p.goLabelsMap == nil {
		return fmt.Errorf("%w: go_labels_procs map not available", errSpanCtxMapError)
	}

	exePath := kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "exe")

	elfFile, err := pfelf.Open(exePath)
	if err != nil {
		return fmt.Errorf("failed to open ELF: %w", err)
	}
	defer elfFile.Close()

	// Detect the Go version from the binary. This reads .go.buildinfo, which
	// survives `-ldflags=-s -w`.
	goVersion, err := elfFile.GoVersion()
	if err != nil {
		return fmt.Errorf("%w: failed to read Go build info: %w", errSpanCtxMalformed, err)
	}
	if goVersion == "" {
		return fmt.Errorf("%w: not a Go binary", errSpanCtxGone)
	}

	parsedGoVersion, ok := goversion.Parse(goVersion)
	if !ok {
		return fmt.Errorf("%w: failed to parse Go version %s", errSpanCtxUnsupported, goVersion)
	}
	// Compare by major.minor only: the runtime layout only changes between minors.
	minorGoVersion := goversion.GoVersion{Major: parsedGoVersion.Major, Minor: parsedGoVersion.Minor}

	// A version newer than the ones the offsets were generated from must be rejected.
	if !minorGoVersion.AfterOrEqual(gooffsets.MinGoVersion) || minorGoVersion.AfterOrEqual(gooffsets.MaxGoVersion) {
		return fmt.Errorf("%w: Go version %s (need >= %s and < %s)", errSpanCtxUnsupported, goVersion, gooffsets.MinGoVersion.String(), gooffsets.MaxGoVersion.String())
	}

	// Get struct offsets from the generated version table.
	offsets, err := gooffsets.GetGoRuntimeOffsets(minorGoVersion, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("%w: %w", errSpanCtxUnsupported, err)
	}

	// Get the TLS G offset by decoding the runtime's own g-load sequence.
	tlsOffset, err := extractTLSGOffset(elfFile)
	switch {
	case err == nil:
	case errors.Is(err, errDecodeSymbol), errors.Is(err, errRuntimeIsCgoUnavailable):
		// Not fatal: extractTLSGOffset still returns the best value it can (the
		// conventional offset on amd64, 0 on arm64). A 0 offset means "g is not
		// in TLS", and eBPF falls back to the g register where the ABI has one.
		seclog.Debugf("Go labels TLS offset for pid %d: %s", pid, err)
	default:
		return fmt.Errorf("%w: failed to extract TLS G offset: %w", errSpanCtxMalformed, err)
	}

	// Serialize and push to BPF map.
	value := serializeGoLabelsOffsets(offsets, tlsOffset)

	if err := p.goLabelsMap.Put(pid, value); err != nil {
		return fmt.Errorf("%w: %w", errSpanCtxMapError, err)
	}
	return nil
}

// serializeGoLabelsOffsets serializes the go_labels_offsets_t struct for the BPF map.
func serializeGoLabelsOffsets(offsets gooffsets.GoRuntimeOffsets, tlsOffset int32) []byte {
	buf := make([]byte, goLabelsOffsetsValueSize)
	binary.NativeEndian.PutUint32(buf[0:4], offsets.MOffset)
	binary.NativeEndian.PutUint32(buf[4:8], offsets.Curg)
	binary.NativeEndian.PutUint32(buf[8:12], offsets.Labels)
	binary.NativeEndian.PutUint32(buf[12:16], offsets.HmapCount)
	binary.NativeEndian.PutUint32(buf[16:20], offsets.HmapLog2BucketCount)
	binary.NativeEndian.PutUint32(buf[20:24], offsets.HmapBuckets)
	binary.NativeEndian.PutUint32(buf[24:28], uint32(tlsOffset))
	return buf
}
