// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	"github.com/go-delve/delve/pkg/goversion"
)

// goLabelsOffsetsValueSize is the serialized size of go_labels_offsets_t:
// 6 * u32 + 1 * s32 = 28 bytes.
const goLabelsOffsetsValueSize = 28

// getGoLabelsOffsets returns the Go runtime struct offsets for pprof label reading,
// based on the Go version. Ported from the OTel eBPF profiler's runtime_data.go.
//
// References:
//   - runtime.g: https://github.com/golang/go/blob/master/src/runtime/runtime2.go
//   - runtime.m: https://github.com/golang/go/blob/master/src/runtime/runtime2.go
//   - runtime.hmap: https://github.com/golang/go/blob/master/src/runtime/map.go
func getGoLabelsOffsets(goVer goversion.GoVersion) (mOffset, curg, labels, hmapCount, hmapLog2BucketCount, hmapBuckets uint32) {
	// m_offset: offset of 'm' field in runtime.g — stable across versions.
	mOffset = 48

	// curg: offset of 'curg' field in runtime.m.
	if goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 25}) {
		curg = 184
	} else {
		curg = 192
	}

	// labels: offset of 'labels' field in runtime.g.
	switch {
	case goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 26}):
		labels = 352
		// Go 1.24+ changed labels from map to slice — signal with hmap_buckets=0.
		return
	case goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 25}):
		labels = 344
		return
	case goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 24}):
		labels = 352
		return
	case goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 23}):
		labels = 352
	case goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 21}):
		labels = 344
	case goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 17}):
		labels = 360
	default:
		labels = 344
	}

	// Go <1.24: labels is a map, need hmap offsets.
	hmapLog2BucketCount = 9
	hmapBuckets = 16
	return
}

// getGoTLSGOffset computes the TLS offset for the G pointer in a Go binary.
// This is the offset from the thread pointer (fsbase on x86_64, tpidr_el0 on ARM64)
// to the runtime.g pointer.
//
// Based on github.com/go-delve/delve/pkg/proc.(*BinaryInfo).setGStructOffsetElf.
func getGoTLSGOffset(elfFile *safeelf.File) (int32, error) {
	var tls *safeelf.Prog
	for _, prog := range elfFile.Progs {
		if prog.Type == safeelf.PT_TLS {
			tls = prog
			break
		}
	}

	switch runtime.GOARCH {
	case "amd64":
		// Look for runtime.tlsg symbol.
		syms, err := elfFile.Symbols()
		if err != nil {
			// Pure Go binary without symbol table: G is at fs-8.
			return -8, nil
		}
		var tlsg *safeelf.Symbol
		for i := range syms {
			if syms[i].Name == "runtime.tlsg" {
				tlsg = &syms[i]
				break
			}
		}
		if tlsg == nil || tls == nil {
			return -8, nil // Pure Go, no cgo: G at fs-8.
		}

		// Linker padding formula (from LLVM lld):
		memsz := tls.Memsz + (-tls.Vaddr-tls.Memsz)&(tls.Align-1)
		// TLS register points to end of TLS block; tlsg is offset from start.
		offset := int64(tlsg.Value) - int64(memsz)
		return int32(offset), nil

	case "arm64":
		syms, err := elfFile.Symbols()
		if err != nil {
			return 16, nil // Default: 2 * pointer_size = 16.
		}
		var tlsg *safeelf.Symbol
		for i := range syms {
			if syms[i].Name == "runtime.tls_g" {
				tlsg = &syms[i]
				break
			}
		}
		if tlsg == nil || tls == nil {
			return 16, nil
		}
		offset := int64(tlsg.Value) + 16 + int64((tls.Vaddr-16)&(tls.Align-1))
		return int32(offset), nil

	default:
		return 0, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

// resolveGoLabels discovers the Go runtime offsets for pprof label reading
// and pushes them to the go_labels_procs BPF map.
func (p *EBPFResolver) resolveGoLabels(pid uint32) error {
	if p.goLabelsMap == nil {
		return fmt.Errorf("go_labels_procs map not available")
	}

	pidStr := strconv.FormatUint(uint64(pid), 10)
	exePath := kernel.HostProc(pidStr, "exe")

	elfFile, err := safeelf.Open(exePath)
	if err != nil {
		return fmt.Errorf("failed to open ELF: %w", err)
	}
	defer elfFile.Close()

	// Detect Go version from the binary.
	goVersionStr, err := binversion.ReadElfBuildInfo(elfFile)
	if err != nil {
		return fmt.Errorf("not a Go binary or failed to read build info: %w", err)
	}

	goVer, ok := goversion.Parse(goVersionStr)
	if !ok {
		return fmt.Errorf("failed to parse Go version: %s", goVersionStr)
	}

	// Minimum supported Go version: 1.13.
	if !goVer.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 13}) {
		return fmt.Errorf("unsupported Go version: %s (need >= 1.13)", goVersionStr)
	}

	// Get struct offsets from version table.
	mOffset, curgOffset, labelsOffset, hmapCount, hmapLog2BC, hmapBuckets := getGoLabelsOffsets(goVer)

	// Get TLS G offset from ELF analysis.
	tlsOffset, err := getGoTLSGOffset(elfFile)
	if err != nil {
		return fmt.Errorf("failed to compute TLS G offset: %w", err)
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
