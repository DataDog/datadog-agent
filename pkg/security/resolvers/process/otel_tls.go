// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"debug/elf" //nolint:depguard
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.opentelemetry.io/ebpf-profiler/libc"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
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

	// otelRuntimeGolang is the Go runtime, which carries thread-level context in
	// pprof labels instead. Never registered: a Go process publishes a process
	// context like any other, but exports no thread-local for this to read, so it
	// resolves to nothing and the pprof label reader has it. Kept as the mirror of
	// OTEL_RUNTIME_GOLANG in pkg/security/ebpf/c/include/constants/enums.h.
	//nolint:unused
	otelRuntimeGolang uint32 = 1
)

// otelTLSValueSize is the serialized size of struct otel_tls_t in
// pkg/security/ebpf/c/include/structs/span_context.h.
const otelTLSValueSize = 32

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
	// attributeKeys is used to name the attribute in the thread context record.
	// This is used to fill Tracer.ThreadlocalAttributeKeys
	attributeKeys []string
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
// the load bias needed to turn its ELF addresses into live ones. The handle is
// a pfelf one because that is what the upstream copy of resolveTLSAccess reads
// relocations through.
type otelTLSModule struct {
	path     string
	loadBias uint64
	file     *pfelf.File
}

// otelTargetProcess holds per-pid state for OTel TLS resolution. The maps
// fields memoize /proc/<pid>/maps, needed both to find the module exporting
// otel_thread_ctx_v1 and to find the libc the DTV layout comes from.
type otelTargetProcess struct {
	pid     uint32
	pidStr  string
	exePath string

	mapsGrouped map[string][]procfs.MapsEntry
	mapsOrder   []string
	mapsErr     error
	mapsDone    bool
	// procCtxAddr is the address of the OTel process context header
	procCtxAddr uint64
}

// resolveOTelTLS prepares the OTel TLS lookup metadata for a process: classify
// otel_thread_ctx_v1's access model from its defining ELF object
// (resolveTLSAccess), then read the loader-resolved GOT/TLSDESC slot from the
// live process (attachOTelTLS). Mirrors the loader/attach split of DataDog's
// opentelemetry-ebpf-profiler fork (PR #1229), collapsed into one call since
// the target here is always already running.
func resolveOTelTLS(pid uint32) (otelTLSResolution, error) {
	target, err := openOTelTargetProcess(pid)
	if err != nil {
		return otelTLSResolution{}, err
	}

	// What a process publishes about itself is what makes it worth resolving,
	// and where the names of its records' attribute indexes come from.
	procCtx, err := target.processContext()
	if err != nil {
		return otelTLSResolution{}, err
	}
	attributeKeys, _ := procCtx.Attributes.StringSlice(otelprocessctx.KeyAttributeKeyMap)

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

	// resolveTLSAccess only reads the symbol's value, but takes the upstream
	// symbol type; the STT_TLS and size checks above are what the rest of it is
	// for, and they need safeelf's Section and Info, which libpf.Symbol has not.
	access, err := resolveTLSAccess(module.file, &libpf.Symbol{
		Address: libpf.SymbolValue(sym.Value),
		Size:    sym.Size,
	})
	if err != nil {
		return otelTLSResolution{}, err
	}

	res, err := attachOTelTLS(target, module.loadBias, access)
	if err != nil {
		return otelTLSResolution{}, err
	}
	res.runtimeLang = otelRuntimeNative
	res.attributeKeys = attributeKeys
	return res, nil
}

// processContext reads the OTel process context of the target, which the maps
// parse the module lookup does anyway has already located.
func (p *otelTargetProcess) processContext() (otelprocessctx.ProcessContext, error) {
	if _, _, err := p.groupedReadableFileMaps(); err != nil {
		return otelprocessctx.ProcessContext{}, err
	}
	if p.procCtxAddr == 0 {
		return otelprocessctx.ProcessContext{}, fmt.Errorf("process %d publishes no OTel process context", p.pid)
	}

	mem, err := procfs.OpenMem(p.pid)
	if err != nil {
		return otelprocessctx.ProcessContext{}, err
	}
	defer mem.Close()

	procCtx, err := otelprocessctx.Read(mem, p.procCtxAddr)
	if err != nil {
		return otelprocessctx.ProcessContext{}, err
	}
	return procCtx, nil
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
		// Since we're already parsing the maps here, set the procCtxAddr
		if p.procCtxAddr == 0 && otelprocessctx.IsMappingName(entry.Pathname) {
			p.procCtxAddr = entry.StartAddr
		}

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

	for _, path := range order {
		fsPath := p.fsPath(path)
		sym := findOTelTLSSymbol(fsPath)
		if sym == nil {
			continue
		}

		elfFile, err := pfelf.Open(fsPath)
		if err != nil {
			continue
		}

		loadBias, err := elfLoadBias(elfFile, grouped[path])
		if err != nil {
			elfFile.Close()
			continue
		}

		return &otelTLSModule{path: path, loadBias: loadBias, file: elfFile}, sym, nil
	}

	return nil, nil, fmt.Errorf("TLS symbol %q not found in currently mapped readable ELF objects", otelTLSSymbolName)
}

// findOTelTLSSymbol looks up otelTLSSymbolName in the object at path: first in
// the dynamic symbol table (exported symbols of shared libraries and PIEs),
// then in the full symbol table (local symbols, fully-static non-PIE
// executables). The lookup stays on safeelf, whose symbols carry the section
// index and the info byte that libpf.Symbol does not.
func findOTelTLSSymbol(path string) *safeelf.Symbol {
	ef, err := openOTelELF(path)
	if err != nil {
		return nil
	}
	defer ef.Close()

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

// attachOTelTLS reads the loader-resolved GOT/TLSDESC slot out of the live
// process and produces the final tls_offset, module_id and dtv_info handed to
// eBPF.
func attachOTelTLS(target *otelTargetProcess, bias uint64, d *data) (otelTLSResolution, error) {
	// Local-exec offsets are final at link time, so this model resolves without
	// reading the target at all.
	if d.access == accessLocalExec {
		return otelTLSResolution{tlsOffset: int64(d.offset)}, nil
	}

	mem, err := procfs.OpenMem(target.pid)
	if err != nil {
		return otelTLSResolution{}, err
	}
	defer mem.Close()

	switch d.access {
	case accessInitialExec:
		// The GOT slot holds the variable's TP-relative offset directly.
		v, err := mem.ReadUint64(bias + uint64(d.elfAddr))
		if err != nil {
			return otelTLSResolution{}, err
		}
		return otelTLSResolution{tlsOffset: int64(v + d.offset)}, nil

	case accessGlobalDynamic:
		// The GOT holds a tls_index {module_id, offset} pair.
		moduleID, err := mem.ReadUint64(bias + uint64(d.elfAddr))
		if err != nil {
			return otelTLSResolution{}, err
		}
		tlsOffset, err := mem.ReadUint64(bias + uint64(d.elfAddr) + 8)
		if err != nil {
			return otelTLSResolution{}, err
		}
		return newDynamicOTelTLS(target, moduleID, tlsOffset+d.offset)

	case accessLocalDynamic:
		// The GOT holds the module_id; the in-module offset is the symbol value.
		moduleID, err := mem.ReadUint64(bias + uint64(d.elfAddr))
		if err != nil {
			return otelTLSResolution{}, err
		}
		return newDynamicOTelTLS(target, moduleID, d.offset)

	case accessTLSDesc:
		// The second word of the descriptor holds the resolved argument.
		arg, err := mem.ReadUint64(bias + uint64(d.elfAddr) + 8)
		if err != nil {
			return otelTLSResolution{}, err
		}

		// For dynamic TLS, arg is a pointer to a tls_index struct. Static
		// offsets are negative on x86_64 and small positive on aarch64, so a
		// large positive value means dynamic.
		if int64(arg) > 0xffffffff {
			moduleID, err := mem.ReadUint64(arg)
			if err != nil {
				return otelTLSResolution{}, err
			}
			tlsOffset, err := mem.ReadUint64(arg + 8)
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

// legacyGlibcLoader matches the loader name glibc used before 2.34, which
// libc.IsPotentialLibcDSO does not: its pattern only knows ld-linux* and
// ld-musl*, while glibc up to 2.33 installs the loader as ld-<version>.so and
// /proc/pid/maps reports that real name rather than the ld-linux-*.so.1
// symlink. __tls_get_addr lives in the loader, so without this every
// pre-2.34 distro -- RHEL/CentOS 7 and 8, Ubuntu 18.04 and 20.04, Amazon
// Linux 2, Debian 10 and 11 -- resolves no DTV at all, and any span published
// through general-dynamic or local-dynamic TLS is silently dropped there.
var legacyGlibcLoader = regexp.MustCompile(`/ld-\d+\.\d+\.so$`)

func isPotentialLibcDSO(path string) bool {
	return libc.IsPotentialLibcDSO(path) || legacyGlibcLoader.MatchString(path)
}

// dtvInfo derives the DTV layout of this process's libc, by disassembling its
// __tls_get_addr through the OTel eBPF profiler's libc package -- the same
// extraction the profiler relies on, rather than a hardcoded table of per-libc,
// per-architecture constants.
func (p *otelTargetProcess) dtvInfo() (otelDTVInfo, error) {
	_, order, err := p.groupedReadableFileMaps()
	if err != nil {
		return otelDTVInfo{}, err
	}

	for _, path := range order {
		if !isPotentialLibcDSO(path) {
			continue
		}
		if info, ok := extractDTVInfo(p.fsPath(path)); ok {
			return info, nil
		}
	}

	// No libc DSO is mapped: a fully-static binary carries libc's code itself.
	if info, ok := extractDTVInfo(p.fsPath(p.exePath)); ok {
		return info, nil
	}

	return otelDTVInfo{}, errors.New("no mapped object exposes a recognizable __tls_get_addr")
}

// extractDTVInfo reads the DTV layout out of one ELF object, reporting false
// when it holds no __tls_get_addr the profiler's decoders recognize.
func extractDTVInfo(path string) (otelDTVInfo, bool) {
	elfFile, err := pfelf.Open(path)
	if err != nil {
		return otelDTVInfo{}, false
	}
	defer elfFile.Close()

	// ExtractLibcInfo also extracts TSD info, which this resolver has no use
	// for, and only fails when both extractions do -- so the DTV half has to be
	// checked on its own.
	info, err := libc.ExtractLibcInfo(elfFile)
	if err != nil || !info.HasDTVInfo() {
		return otelDTVInfo{}, false
	}
	// Widened to the field types struct otel_dtv_info_t needs; see otelDTVInfo.
	return otelDTVInfo{
		offset:     int64(info.DTVInfo.Offset),
		multiplier: uint32(info.DTVInfo.Multiplier),
	}, true
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
// runtime addresses: one of its executable mappings is turned back into the
// virtual address the object was linked at, and the bias is that mapping's
// start address minus it. The conversion, including the page-alignment
// subtleties of how the kernel and the dynamic loader place a segment, comes
// from the profiler's pfelf.AddressMapper; it indexes executable PT_LOAD
// segments only, hence the executable anchor.
func elfLoadBias(elfFile *pfelf.File, maps []procfs.MapsEntry) (uint64, error) {
	if elfFile.Type == elf.ET_EXEC {
		return 0, nil
	}

	anchor, ok := executableMapping(maps)
	if !ok {
		return 0, errors.New("no executable mapping for ELF")
	}

	mapper := elfFile.GetAddressMapper()
	vaddr, ok := mapper.FileOffsetToVirtualAddress(anchor.Offset)
	if !ok {
		return 0, fmt.Errorf("no executable segment covers file offset %#x", anchor.Offset)
	}

	if anchor.StartAddr < vaddr {
		return 0, fmt.Errorf("mapped at %#x, below the link-time address %#x", anchor.StartAddr, vaddr)
	}
	return anchor.StartAddr - vaddr, nil
}

func executableMapping(maps []procfs.MapsEntry) (procfs.MapsEntry, bool) {
	for _, entry := range maps {
		if strings.Contains(entry.Permissions, "x") {
			return entry, true
		}
	}
	return procfs.MapsEntry{}, false
}
