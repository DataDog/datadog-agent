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
	"go/version"
	"strconv"

	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// goLabelsOffsetsValueSize is the serialized size of go_labels_offsets_t:
// 6 * u32 + 1 * s32 = 28 bytes.
const goLabelsOffsetsValueSize = 28

// Supported Go version range. The struct offsets below are hand-maintained per
// release, so a version we have never seen must be rejected rather than silently
// read with the offsets of an older one.
const (
	minGoVersion = "go1.13"
	// maxGoVersion is exclusive.
	maxGoVersion = "go1.28"
)

var (
	// errDecodeSymbol is returned when the TLS offset could not be recovered from
	// the instruction stream of the symbol we decode.
	errDecodeSymbol = errors.New("failed to decode symbol")
	// errRuntimeIsCgoUnavailable is returned when runtime.iscgo could not be read
	// out of the binary, so we cannot tell whether the runtime keeps g in TLS.
	errRuntimeIsCgoUnavailable = errors.New("runtime.iscgo value unavailable")
)

// getGoLabelsOffsets returns the Go runtime struct offsets for pprof label reading,
// based on the Go version. Kept in sync with the OTel eBPF profiler's
// interpreter/go/offsets.go (DataDog fork); update both together.
//
// References:
//   - runtime.g: https://github.com/golang/go/blob/master/src/runtime/runtime2.go
//   - runtime.m: https://github.com/golang/go/blob/master/src/runtime/runtime2.go
//   - runtime.hmap: https://github.com/golang/go/blob/master/src/runtime/map.go
func getGoLabelsOffsets(goVersion string) (mOffset, curg, labels, hmapCount, hmapLog2BucketCount, hmapBuckets uint32) {
	// m_offset: offset of 'm' field in runtime.g — stable across versions.
	mOffset = 48

	// curg: offset of 'curg' field in runtime.m.
	curg = 192
	if version.Compare(goVersion, "go1.25") >= 0 {
		curg = 184
	}

	// labels: offset of 'labels' field in runtime.g.
	// Go 1.24+ changed labels from a map to a slice — signal that to eBPF by
	// leaving hmap_buckets at 0.
	switch {
	case version.Compare(goVersion, "go1.26") >= 0:
		labels = 352
		return
	case version.Compare(goVersion, "go1.25") >= 0:
		labels = 344
		return
	case version.Compare(goVersion, "go1.24") >= 0:
		labels = 352
		return
	}

	// Go <1.24: labels is a map, need hmap offsets.
	hmapLog2BucketCount = 9
	hmapBuckets = 16

	switch {
	case version.Compare(goVersion, "go1.23") >= 0:
		labels = 352
	case version.Compare(goVersion, "go1.21") >= 0:
		labels = 344
	case version.Compare(goVersion, "go1.17") >= 0:
		labels = 360
	default:
		labels = 344
	}
	return
}

// resolveGoLabels discovers the Go runtime offsets for pprof label reading
// and pushes them to the go_labels_procs BPF map.
func (p *EBPFResolver) resolveGoLabels(pid uint32) error {
	if p.goLabelsMap == nil {
		return errors.New("go_labels_procs map not available")
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
		return fmt.Errorf("failed to read Go build info: %w", err)
	}
	if goVersion == "" {
		return errors.New("not a Go binary")
	}

	if version.Compare(goVersion, minGoVersion) < 0 || version.Compare(goVersion, maxGoVersion) >= 0 {
		return fmt.Errorf("unsupported Go version %s (need >= %s and < %s)", goVersion, minGoVersion, maxGoVersion)
	}

	// Get struct offsets from the version table.
	mOffset, curgOffset, labelsOffset, hmapCount, hmapLog2BC, hmapBuckets := getGoLabelsOffsets(goVersion)

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
		return fmt.Errorf("failed to extract TLS G offset: %w", err)
	}

	// Serialize and push to BPF map.
	value := serializeGoLabelsOffsets(mOffset, curgOffset, labelsOffset,
		hmapCount, hmapLog2BC, hmapBuckets, tlsOffset)

	return p.goLabelsMap.Put(pid, value)
}

// serializeGoLabelsOffsets serializes the go_labels_offsets_t struct for the BPF map.
func serializeGoLabelsOffsets(mOffset, curg, labels, hmapCount, hmapLog2BC, hmapBuckets uint32, tlsOffset int32) []byte {
	buf := make([]byte, goLabelsOffsetsValueSize)
	binary.NativeEndian.PutUint32(buf[0:4], mOffset)
	binary.NativeEndian.PutUint32(buf[4:8], curg)
	binary.NativeEndian.PutUint32(buf[8:12], labels)
	binary.NativeEndian.PutUint32(buf[12:16], hmapCount)
	binary.NativeEndian.PutUint32(buf[16:20], hmapLog2BC)
	binary.NativeEndian.PutUint32(buf[20:24], hmapBuckets)
	binary.NativeEndian.PutUint32(buf[24:28], uint32(tlsOffset))
	return buf
}
