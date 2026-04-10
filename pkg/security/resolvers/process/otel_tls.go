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

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	// otelTLSSymbolName is the TLS symbol name defined by OTel spec PR #4947.
	otelTLSSymbolName = "otel_thread_ctx_v1"

	// otelRuntimeNative represents a native runtime (C, C++, Rust, Java/JNI, etc.)
	// that uses ELF TLSDESC for thread-local storage.
	otelRuntimeNative uint32 = 0

	// otelRuntimeGolang represents the Go runtime, which uses a different mechanism
	// (pprof labels) for thread-level context. Not yet supported.
	otelRuntimeGolang uint32 = 1

	// otelTLSValueSize is the serialized size of otel_tls_t: s64 + u32 + u32.
	otelTLSValueSize = 16
)

// mapTracerLanguageToRuntime maps the tracer language string from TracerMetadata
// to the otel_runtime_language enum used in the BPF map.
func mapTracerLanguageToRuntime(tracerLanguage string) uint32 {
	switch tracerLanguage {
	case "go":
		return otelRuntimeGolang
	default:
		return otelRuntimeNative
	}
}

// findOTelTLSSymbol searches an ELF file for the otel_thread_ctx_v1 TLS symbol.
// It first tries DynamicSymbols (.dynsym), then falls back to Symbols (.symtab).
func findOTelTLSSymbol(elfFile *safeelf.File) (*safeelf.Symbol, error) {
	// Try dynamic symbols first (always present in shared libraries).
	syms, err := elfFile.DynamicSymbols()
	if err == nil {
		for i := range syms {
			if syms[i].Name == otelTLSSymbolName && safeelf.ST_TYPE(syms[i].Info) == safeelf.STT_TLS {
				return &syms[i], nil
			}
		}
	}

	// Fall back to static symbol table (present in unstripped binaries).
	syms, err = elfFile.Symbols()
	if err == nil {
		for i := range syms {
			if syms[i].Name == otelTLSSymbolName && safeelf.ST_TYPE(syms[i].Info) == safeelf.STT_TLS {
				return &syms[i], nil
			}
		}
	}

	return nil, fmt.Errorf("TLS symbol %q not found", otelTLSSymbolName)
}

// alignUp rounds v up to the nearest multiple of align.
func alignUp(v, align uint64) uint64 {
	return (v + align - 1) &^ (align - 1)
}

// computeStaticTLSOffset computes the static TLS offset for a symbol, given the
// ELF file that contains it. The offset is relative to the thread pointer.
//
// x86_64: TLS block is below the thread pointer → negative offset.
// ARM64:  TLS block is above the thread pointer with a 16-byte TCB gap → positive offset.
func computeStaticTLSOffset(sym *safeelf.Symbol, elfFile *safeelf.File) (int64, error) {
	// Find the PT_TLS program header.
	var tlsSeg *safeelf.Prog
	for _, prog := range elfFile.Progs {
		if prog.Type == safeelf.PT_TLS {
			tlsSeg = prog
			break
		}
	}
	if tlsSeg == nil {
		return 0, errors.New("no PT_TLS segment found")
	}

	switch runtime.GOARCH {
	case "amd64":
		// x86_64 variant II TLS: TLS block placed below the thread pointer.
		// offset = sym.Value - alignUp(memsz, align)
		memsz := alignUp(tlsSeg.Memsz, tlsSeg.Align)
		return int64(sym.Value) - int64(memsz), nil
	case "arm64":
		// ARM64 variant I TLS: TLS block placed above the thread pointer with 16-byte TCB.
		return int64(sym.Value) + 16, nil
	default:
		return 0, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

// resolveOTelTLS discovers the OTel TLS symbol for a process and computes the
// static TLS offset. It searches the main executable first, then loaded shared
// libraries via /proc/<pid>/maps.
func resolveOTelTLS(pid uint32, tracerLanguage string) (int64, uint32, error) {
	runtimeLang := mapTracerLanguageToRuntime(tracerLanguage)

	// For Go runtimes, we still register the entry (with the language tag) so that
	// the eBPF side can differentiate, but we don't need to resolve the TLS symbol
	// since Go doesn't use ELF TLSDESC.
	if runtimeLang == otelRuntimeGolang {
		return 0, runtimeLang, nil
	}

	pidStr := strconv.FormatUint(uint64(pid), 10)

	// Try the main executable first.
	exePath := kernel.HostProc(pidStr, "exe")
	offset, err := findTLSOffsetInFile(exePath)
	if err == nil {
		return offset, runtimeLang, nil
	}

	// Fall back to searching loaded shared libraries via /proc/<pid>/maps.
	mappedFiles, mapErr := procfs.GetMappedFiles(int32(pid), 0, procfs.FilterExecutableRegularFiles)
	if mapErr != nil {
		return 0, 0, fmt.Errorf("symbol not in executable (%w), and failed to read maps: %w", err, mapErr)
	}

	for _, libPath := range mappedFiles {
		// Access the library through the process's root filesystem.
		hostLibPath := kernel.HostProc(pidStr, "root", libPath)
		offset, libErr := findTLSOffsetInFile(hostLibPath)
		if libErr == nil {
			return offset, runtimeLang, nil
		}
	}

	return 0, 0, fmt.Errorf("TLS symbol %q not found in executable or any loaded library", otelTLSSymbolName)
}

// findTLSOffsetInFile opens an ELF file, searches for the OTel TLS symbol, and
// computes the static TLS offset.
func findTLSOffsetInFile(path string) (int64, error) {
	elfFile, err := safeelf.Open(path)
	if err != nil {
		return 0, err
	}
	defer elfFile.Close()

	sym, err := findOTelTLSSymbol(elfFile)
	if err != nil {
		return 0, err
	}

	return computeStaticTLSOffset(sym, elfFile)
}

// serializeOTelTLSValue serializes the otel_tls_t struct for the BPF map.
// Layout: s64 tls_offset (8 bytes) + u32 runtime (4 bytes) + u32 _pad (4 bytes)
func serializeOTelTLSValue(offset int64, runtimeLang uint32) []byte {
	buf := make([]byte, otelTLSValueSize)
	binary.NativeEndian.PutUint64(buf[0:8], uint64(offset))
	binary.NativeEndian.PutUint32(buf[8:12], runtimeLang)
	// buf[12:16] is padding, already zero
	return buf
}
