// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"bytes"
	"debug/elf" //nolint:depguard
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	// otelTLSSymbolName is the TLS symbol name defined by OTel spec PR #4947.
	otelTLSSymbolName = "otel_thread_ctx_v1"
	// otelTLSExportSize is the expected size of otel_thread_ctx_v1: it holds a
	// pointer to the active Thread Local Context Record, not the record itself.
	otelTLSExportSize = 8

	// otelRuntimeNative is a runtime using ELF thread-local storage (C, C++,
	// Rust, Java/JNI, ...).
	otelRuntimeNative uint32 = 0

	// otelRuntimeGolang is the Go runtime, which carries thread-level context
	// in pprof labels instead.
	otelRuntimeGolang uint32 = 1
)

// otelTLSValueSize is the serialized size of struct otel_tls_t in
// pkg/security/ebpf/c/include/structs/span_context.h.
const otelTLSValueSize = 32

// mapTracerLanguageToRuntime maps a TracerMetadata language to the
// otel_runtime_language enum.
func mapTracerLanguageToRuntime(tracerLanguage string) uint32 {
	switch tracerLanguage {
	case "go":
		return otelRuntimeGolang
	default:
		return otelRuntimeNative
	}
}

// otelDTVInfo describes how to walk the Dynamic Thread Vector (DTV) for a
// process's libc. The signed fields here and in otelTLSResolution must stay
// 64-bit to match struct otel_dtv_info_t / otel_tls_t in
// structs/span_context.h, which explains why.
type otelDTVInfo struct {
	// offset is the offset of the DTV pointer from the thread pointer base.
	offset int64
	// multiplier is the size of each DTV entry in bytes (16 glibc / 8 musl).
	multiplier uint32
}

// otelTLSResolution is everything eBPF needs to read otel_thread_ctx_v1 for
// one process at trace time; serialized as struct otel_tls_t.
type otelTLSResolution struct {
	// runtimeLang is the otel_runtime_language enum: Go processes use the
	// pprof-labels reader (go_labels.go) rather than TLS.
	runtimeLang uint32
	// moduleID is the TLS module ID for dynamic TLS, or 0 for static TLS.
	moduleID uint32
	// tlsOffset is TP-relative (static TLS, moduleID == 0) or the offset
	// within the module's TLS block (dynamic TLS, moduleID != 0).
	tlsOffset int64
	// dtvInfo locates the DTV for dynamic TLS (unused when moduleID == 0).
	dtvInfo otelDTVInfo
}

// serializeOTelTLSValue serializes res as struct otel_tls_t.
func serializeOTelTLSValue(res otelTLSResolution) []byte {
	buf := make([]byte, otelTLSValueSize)
	binary.NativeEndian.PutUint32(buf[0:4], res.runtimeLang)
	binary.NativeEndian.PutUint32(buf[4:8], res.moduleID)
	binary.NativeEndian.PutUint64(buf[8:16], uint64(res.tlsOffset))
	binary.NativeEndian.PutUint64(buf[16:24], uint64(res.dtvInfo.offset))
	binary.NativeEndian.PutUint32(buf[24:28], res.dtvInfo.multiplier)
	// buf[28:32] is dtv_info._pad, intentionally left zero.
	return buf
}

// otelTLSModule is the mapped ELF object exporting otel_thread_ctx_v1, with
// the load bias needed to turn its ELF addresses into live ones.
type otelTLSModule struct {
	path     string
	loadBias uint64
	file     *safeelf.File
}

// otelTargetProcess holds per-pid state for OTel TLS resolution. The maps
// fields memoize /proc/<pid>/maps, needed both to find the module exporting
// otel_thread_ctx_v1 and to detect musl vs. glibc.
type otelTargetProcess struct {
	pid     uint32
	pidStr  string
	exePath string

	mapsGrouped map[string][]procfs.MapsEntry
	mapsOrder   []string
	mapsErr     error
	mapsDone    bool
}

// resolveOTelTLS prepares the OTel TLS lookup metadata for a process: classify
// otel_thread_ctx_v1's access model from its defining ELF object
// (resolveTLSAccess), then read the loader-resolved GOT/TLSDESC slot from the
// live process (attachOTelTLS). Mirrors the loader/attach split of DataDog's
// opentelemetry-ebpf-profiler fork (PR #1229), collapsed into one call since
// the target here is always already running.
func resolveOTelTLS(pid uint32, tracerLanguage string) (otelTLSResolution, error) {
	runtimeLang := mapTracerLanguageToRuntime(tracerLanguage)
	if runtimeLang == otelRuntimeGolang {
		return otelTLSResolution{runtimeLang: runtimeLang}, nil
	}

	target, err := openOTelTargetProcess(pid)
	if err != nil {
		return otelTLSResolution{}, err
	}

	module, sym, err := target.findOTelTLSModule()
	if err != nil {
		return otelTLSResolution{}, err
	}
	defer module.file.Close()

	if sym.Size != otelTLSExportSize {
		return otelTLSResolution{}, fmt.Errorf("TLS export has wrong size %d", sym.Size)
	}
	if safeelf.ST_TYPE(sym.Info) != elf.STT_TLS {
		return otelTLSResolution{}, errors.New("TLS export is not a TLS symbol")
	}

	access, err := resolveTLSAccess(module.file, sym)
	if err != nil {
		return otelTLSResolution{}, err
	}

	res, err := attachOTelTLS(target, module.loadBias, access)
	if err != nil {
		return otelTLSResolution{}, err
	}
	res.runtimeLang = runtimeLang
	return res, nil
}

func openOTelTargetProcess(pid uint32) (*otelTargetProcess, error) {
	pidStr := strconv.FormatUint(uint64(pid), 10)
	exePath, err := os.Readlink(kernel.HostProc(pidStr, "exe"))
	if err != nil {
		return nil, fmt.Errorf("resolve /proc/%s/exe: %w", pidStr, err)
	}
	exePath = stripDeletedMapsSuffix(exePath)

	return &otelTargetProcess{
		pid:     pid,
		pidStr:  pidStr,
		exePath: exePath,
	}, nil
}

func (p *otelTargetProcess) fsPath(path string) string {
	return kernel.HostProc(p.pidStr, "root", path)
}

func (p *otelTargetProcess) maps() ([]procfs.MapsEntry, error) {
	mapsPath := kernel.HostProc(p.pidStr, "maps")
	file, err := os.Open(mapsPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mapsPath, err)
	}
	defer file.Close()

	var entries []procfs.MapsEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if entry, ok := procfs.ParseMapsLine(scanner.Bytes()); ok {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", mapsPath, err)
	}
	return entries, nil
}

func (p *otelTargetProcess) groupedReadableFileMaps() (map[string][]procfs.MapsEntry, []string, error) {
	if p.mapsDone {
		return p.mapsGrouped, p.mapsOrder, p.mapsErr
	}
	p.mapsDone = true
	p.mapsGrouped, p.mapsOrder, p.mapsErr = p.computeGroupedReadableFileMaps()
	return p.mapsGrouped, p.mapsOrder, p.mapsErr
}

func (p *otelTargetProcess) computeGroupedReadableFileMaps() (map[string][]procfs.MapsEntry, []string, error) {
	entries, err := p.maps()
	if err != nil {
		return nil, nil, err
	}

	grouped := make(map[string][]procfs.MapsEntry)
	var order []string
	seen := make(map[string]struct{})
	for _, entry := range entries {
		path, ok := otelReadableFileMappingPath(entry)
		if !ok {
			continue
		}
		grouped[path] = append(grouped[path], entry)
		if _, ok := seen[path]; !ok {
			seen[path] = struct{}{}
			order = append(order, path)
		}
	}
	return grouped, order, nil
}

// otelReadableFileMappingPath returns the cleaned pathname of a readable,
// file-backed mapping, or false for anonymous/special/non-readable mappings.
func otelReadableFileMappingPath(e procfs.MapsEntry) (string, bool) {
	path := stripDeletedMapsSuffix(e.Pathname)
	if path == "" || path[0] != '/' || !strings.HasPrefix(e.Permissions, "r") {
		return "", false
	}
	return path, true
}

func stripDeletedMapsSuffix(path string) string {
	return strings.TrimSuffix(path, " (deleted)")
}

// findOTelTLSModule returns the first mapped, readable ELF object exporting an
// otel_thread_ctx_v1 STT_TLS symbol. The returned ELF file is left open for
// resolveTLSAccess, which reads the same object's relocations; callers must
// Close() it.
func (p *otelTargetProcess) findOTelTLSModule() (*otelTLSModule, *safeelf.Symbol, error) {
	grouped, order, err := p.groupedReadableFileMaps()
	if err != nil {
		return nil, nil, err
	}

	pageSize := uint64(os.Getpagesize())
	for _, path := range order {
		elfFile, err := openOTelELF(p.fsPath(path))
		if err != nil {
			continue
		}

		sym := findOTelTLSSymbol(elfFile)
		if sym == nil {
			elfFile.Close()
			continue
		}

		loadBias, err := elfLoadBias(elfFile, grouped[path], pageSize)
		if err != nil {
			elfFile.Close()
			continue
		}

		return &otelTLSModule{path: path, loadBias: loadBias, file: elfFile}, sym, nil
	}

	return nil, nil, fmt.Errorf("TLS symbol %q not found in currently mapped readable ELF objects", otelTLSSymbolName)
}

// findOTelTLSSymbol looks up otelTLSSymbolName in the dynamic symbol table
// (exported symbols of shared libraries and PIEs), then in the full symbol
// table (local symbols, fully-static non-PIE executables).
func findOTelTLSSymbol(ef *safeelf.File) *safeelf.Symbol {
	if syms, err := ef.DynamicSymbols(); err == nil {
		if sym := findOTelTLSSymbolByName(syms); sym != nil {
			return sym
		}
	}
	if syms, err := ef.Symbols(); err == nil {
		if sym := findOTelTLSSymbolByName(syms); sym != nil {
			return sym
		}
	}
	return nil
}

func findOTelTLSSymbolByName(syms []safeelf.Symbol) *safeelf.Symbol {
	for i := range syms {
		sym := &syms[i]
		if sym.Name == otelTLSSymbolName && sym.Section != elf.SHN_UNDEF &&
			safeelf.ST_TYPE(sym.Info) == elf.STT_TLS {
			return sym
		}
	}
	return nil
}

// attachOTelTLS reads the loader-resolved GOT/TLSDESC slot from the live
// process (/proc/<pid>/mem) and produces the final tls_offset, module_id and
// dtv_info handed to eBPF.
func attachOTelTLS(target *otelTargetProcess, bias uint64, d *otelTLSAccessData) (otelTLSResolution, error) {
	rm := otelRemoteMemory{pidStr: target.pidStr}

	switch d.access {
	case accessLocalExec:
		return otelTLSResolution{tlsOffset: int64(d.offset)}, nil

	case accessInitialExec:
		// The GOT slot holds the variable's TP-relative offset directly.
		v, err := rm.Uint64(bias + d.elfAddr)
		if err != nil {
			return otelTLSResolution{}, err
		}
		return otelTLSResolution{tlsOffset: int64(v + d.offset)}, nil

	case accessGlobalDynamic:
		// The GOT holds a tls_index {module_id, offset} pair.
		moduleID, err := rm.Uint64(bias + d.elfAddr)
		if err != nil {
			return otelTLSResolution{}, err
		}
		tlsOffset, err := rm.Uint64(bias + d.elfAddr + 8)
		if err != nil {
			return otelTLSResolution{}, err
		}
		return newDynamicOTelTLS(target, moduleID, tlsOffset+d.offset)

	case accessLocalDynamic:
		// The GOT holds the module_id; the in-module offset is the symbol value.
		moduleID, err := rm.Uint64(bias + d.elfAddr)
		if err != nil {
			return otelTLSResolution{}, err
		}
		return newDynamicOTelTLS(target, moduleID, d.offset)

	case accessTLSDesc:
		// The second word of the descriptor holds the resolved argument.
		arg, err := rm.Uint64(bias + d.elfAddr + 8)
		if err != nil {
			return otelTLSResolution{}, err
		}

		// For dynamic TLS, arg is a pointer to a tls_index struct. Static
		// offsets are negative on x86_64 and small positive on aarch64, so a
		// large positive value means dynamic.
		if int64(arg) > 0xffffffff {
			moduleID, err := rm.Uint64(arg)
			if err != nil {
				return otelTLSResolution{}, err
			}
			tlsOffset, err := rm.Uint64(arg + 8)
			if err != nil {
				return otelTLSResolution{}, err
			}
			return newDynamicOTelTLS(target, moduleID, tlsOffset+d.offset)
		}
		return otelTLSResolution{tlsOffset: int64(arg + d.offset)}, nil

	default:
		return otelTLSResolution{}, fmt.Errorf("unknown TLS access model %v", d.access)
	}
}

// newDynamicOTelTLS fills in the DTVInfo needed to walk this process's DTV.
func newDynamicOTelTLS(target *otelTargetProcess, moduleID, tlsOffset uint64) (otelTLSResolution, error) {
	if moduleID == 0 {
		return otelTLSResolution{}, errors.New("unexpected value 0 for moduleID in dynamic TLS")
	}
	dtv, err := target.dtvInfo()
	if err != nil {
		return otelTLSResolution{}, err
	}
	return otelTLSResolution{
		tlsOffset: int64(tlsOffset),
		moduleID:  uint32(moduleID),
		dtvInfo:   dtv,
	}, nil
}

// otelRemoteMemory reads absolute virtual addresses out of a live process's
// memory via /proc/<pid>/mem.
type otelRemoteMemory struct {
	pidStr string
}

// Uint64 reads one 8-byte native-endian word at addr. The target can exit
// between symbol resolution and this read, so errors here are expected
// resolution failures, not bugs.
func (rm otelRemoteMemory) Uint64(addr uint64) (uint64, error) {
	memPath := kernel.HostProc(rm.pidStr, "mem")
	f, err := os.Open(memPath)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", memPath, err)
	}
	defer f.Close()

	var buf [8]byte
	if _, err := f.ReadAt(buf[:], int64(addr)); err != nil {
		return 0, fmt.Errorf("read 8 bytes at %#x from %s: %w", addr, memPath, err)
	}
	return binary.NativeEndian.Uint64(buf[:]), nil
}

func (p *otelTargetProcess) dtvInfo() (otelDTVInfo, error) {
	musl, err := p.usesMusl()
	if err != nil {
		return otelDTVInfo{}, err
	}
	return defaultOTelDTVInfo(musl), nil
}

// usesMusl reports whether the process's dynamic loader (or, for fully-static
// binaries, its libc) is musl rather than glibc, which is all that is needed
// to pick the DTV layout.
func (p *otelTargetProcess) usesMusl() (bool, error) {
	_, order, err := p.groupedReadableFileMaps()
	if err != nil {
		return false, err
	}
	for _, path := range order {
		if isMuslLoaderPath(path) {
			return true, nil
		}
	}

	// No musl loader mapped: look for a musl PT_INTERP, or the main_tls /
	// builtin_tls .symtab marker pair that fully-static musl binaries carry.
	elfFile, err := openOTelELF(p.fsPath(p.exePath))
	if err != nil {
		return false, err
	}
	defer elfFile.Close()

	if isMuslLoaderPath(elfInterpreter(elfFile)) {
		return true, nil
	}
	_, mainTLS := symbolValueInSymtab(elfFile, "main_tls")
	_, builtinTLS := symbolValueInSymtab(elfFile, "builtin_tls")
	return mainTLS && builtinTLS, nil
}

func isMuslLoaderPath(path string) bool {
	return strings.Contains(path, "/ld-musl-")
}

// defaultOTelDTVInfo returns the DTV entry size and TCB-to-DTV-pointer offset
// for the given libc on the running architecture, hardcoded rather than
// derived by disassembling __tls_get_addr the way upstream does.
//
// TODO(OTEP-4947): the x86_64 musl offset is unverified against musl's struct
// pthread layout (src/internal/pthread_impl.h) and just reuses the glibc
// value; no test covers musl + dlopen'd TLS.
func defaultOTelDTVInfo(musl bool) otelDTVInfo {
	tcbDTVOffset := int64(0)
	switch runtime.GOARCH {
	case "amd64":
		// glibc tcbhead_t{void *tcb; dtv_t *dtv; ...}; tp points at tcb, which
		// aliases the struct itself, so dtv sits at tp+8.
		tcbDTVOffset = 8
	case "arm64":
		if musl {
			tcbDTVOffset = -8
		}
		// else: glibc tcbhead_t{void *dtv; void *private}; tp (TPIDR_EL0)
		// points directly at the struct, so dtv sits at tp+0.
	}

	entrySize := uint32(16) // glibc dtv_t{counter, pointer} pairs
	if musl {
		entrySize = 8 // musl's dtv is a plain void* array, no generation-counter word
	}

	return otelDTVInfo{offset: tcbDTVOffset, multiplier: entrySize}
}

func openOTelELF(path string) (*safeelf.File, error) {
	elfFile, err := safeelf.Open(path)
	if err != nil {
		return nil, err
	}
	if elfFile.Class != elf.ELFCLASS64 || elfFile.Data != elf.ELFDATA2LSB {
		elfFile.Close()
		return nil, fmt.Errorf("unsupported ELF class/data in %s", path)
	}
	if elfFile.Machine != elf.EM_X86_64 && elfFile.Machine != elf.EM_AARCH64 {
		elfFile.Close()
		return nil, fmt.Errorf("unsupported ELF machine %v in %s", elfFile.Machine, path)
	}
	return elfFile, nil
}

// elfLoadBias returns the difference between an ELF object's link-time and
// runtime addresses: its lowest mapping is matched to the PT_LOAD segment with
// the same page-aligned file offset, and the bias is that mapping's start
// address minus the segment's page-aligned vaddr.
func elfLoadBias(elfFile *safeelf.File, maps []procfs.MapsEntry, pageSize uint64) (uint64, error) {
	if elfFile.Type == elf.ET_EXEC {
		return 0, nil
	}
	if len(maps) == 0 {
		return 0, errors.New("no maps entries for ELF")
	}

	anchor := maps[0]
	for _, entry := range maps[1:] {
		if entry.StartAddr < anchor.StartAddr {
			anchor = entry
		}
	}

	for _, prog := range elfFile.Progs {
		if prog.Type != elf.PT_LOAD {
			continue
		}

		phdrOffset := alignDown(prog.Off, pageSize)
		phdrVaddr := alignDown(prog.Vaddr, pageSize)
		if anchor.Offset == phdrOffset && anchor.StartAddr >= phdrVaddr {
			return anchor.StartAddr - phdrVaddr, nil
		}
	}

	return 0, errors.New("could not compute load bias")
}

func alignDown(value uint64, alignment uint64) uint64 {
	if alignment <= 1 {
		return value
	}
	return value &^ (alignment - 1)
}

func elfInterpreter(elfFile *safeelf.File) string {
	for _, prog := range elfFile.Progs {
		if prog.Type != elf.PT_INTERP || prog.Filesz == 0 {
			continue
		}
		data, err := readELFProgBytes(prog, 0, int(prog.Filesz))
		if err != nil {
			return ""
		}
		if idx := bytes.IndexByte(data, 0); idx >= 0 {
			data = data[:idx]
		}
		return string(data)
	}
	return ""
}

func readELFProgBytes(prog *elf.Prog, offset uint64, size int) ([]byte, error) {
	if size < 0 || offset > prog.Filesz || uint64(size) > prog.Filesz-offset {
		return nil, io.ErrUnexpectedEOF
	}

	reader := prog.Open()
	if _, err := reader.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, err
	}

	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	return data, err
}

func symbolValueInSymtab(elfFile *safeelf.File, name string) (uint64, bool) {
	syms, err := elfFile.Symbols()
	if err != nil {
		return 0, false
	}
	for _, sym := range syms {
		if sym.Name == name && sym.Section != elf.SHN_UNDEF {
			return sym.Value, true
		}
	}
	return 0, false
}
