// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	// otelTLSSymbolName is the TLS symbol name defined by OTel spec PR #4947.
	otelTLSSymbolName = "otel_thread_ctx_v1"

	// otelRuntimeNative represents a native runtime (C, C++, Rust, Java/JNI, etc.)
	// that uses ELF thread-local storage.
	otelRuntimeNative uint32 = 0

	// otelRuntimeGolang represents the Go runtime, which uses a different mechanism
	// (pprof labels) for thread-level context.
	otelRuntimeGolang uint32 = 1
)

const (
	otelTLSModeStaticMain uint32 = 1
	otelTLSModeLinkMap    uint32 = 2
)

const (
	otelTLSMaxModules    = 256
	otelTLSHashSlots     = 8192
	otelTLSHashWords     = otelTLSHashSlots / 64
	otelTLSMaxSeedSearch = 1 << 20

	otelTLSHashMul1 = uint64(0xff51afd7ed558ccd)
	otelTLSHashMul2 = uint64(0xc4ceb9fe1a85ec53)
)

const (
	// 64-bit glibc/musl public loader layouts used by the tls-modid-bpf sample.
	rDebugRMapOffset        uint64 = 8
	linkMapLAddrOffset      uint64 = 0
	linkMapLNextOffset      uint64 = 24
	glibcLinkMapLRealOffset uint64 = 40
)

// otelTLSValueSize is the serialized size of struct otel_tls_t in
// pkg/security/ebpf/c/include/structs/process.h.
const otelTLSValueSize = 120 + otelTLSHashWords*8 + 48

type otelLoaderKind uint8

const (
	otelLoaderGlibc otelLoaderKind = iota
	otelLoaderMusl
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

// otelTLSResolution is a prepared lookup request for the eBPF side. It is built
// only from ELF files and procfs metadata. Live target memory is read by eBPF.
type otelTLSResolution struct {
	dtDebugValueAddr          uint64
	targetLoadBias            uint64
	targetSymbolOffset        uint64
	targetSymbolSize          uint64
	targetTLSMemsz            uint64
	rDebugRMapOffset          uint64
	linkMapLAddrOffset        uint64
	linkMapLNextOffset        uint64
	linkMapLRealOffset        uint64
	linkMapLTLSModIDOffset    uint64
	linkMapLTLSTPOffsetOffset uint64
	tcbDTVOffset              int64
	dtvEntrySize              uint64
	dtvEntryPointerOffset     uint64
	tlsModuleHashSeed         uint64
	tlsModuleHashBits         [otelTLSHashWords]uint64
	mode                      uint32
	reconstructModuleIDs      uint32
	tlsModuleCount            uint32
	runtimeLang               uint32
}

type otelDTVLayout struct {
	tcbDTVOffset          int64
	dtvEntrySize          uint64
	dtvEntryPointerOffset uint64
}

type otelLinkMapTLSLayout struct {
	modIDOffset     uint64
	tlsOffsetOffset uint64
}

type otelGlibcThreadDBLayout struct {
	linkMap otelLinkMapTLSLayout
	dtv     otelDTVLayout
}

type otelTLSModuleSet struct {
	seed uint64
	bits [otelTLSHashWords]uint64
}

type otelAuxvInfo struct {
	atBase uint64
}

type otelMapEntry struct {
	start  uint64
	end    uint64
	offset uint64
	perms  string
	path   string
}

type otelModuleImage struct {
	path     string
	loadBias uint64
	hasTLS   bool
}

type otelTLSSymbol struct {
	name   string
	offset uint64
	size   uint64
}

type otelTLSCandidate struct {
	path           string
	loadBias       uint64
	symbolOffset   uint64
	symbolSize     uint64
	tlsMemsz       uint64
	mainExecutable bool
}

type otelTargetProcess struct {
	pid     uint32
	pidStr  string
	exePath string
	auxv    otelAuxvInfo
}

// resolveOTelTLS prepares the OTel TLS lookup metadata for a process by
// mirroring the userspace side of samples/tls-modid-bpf. It does not read
// /proc/<pid>/mem.
func resolveOTelTLS(pid uint32, tracerLanguage string) (otelTLSResolution, error) {
	runtimeLang := mapTracerLanguageToRuntime(tracerLanguage)
	if runtimeLang == otelRuntimeGolang {
		return otelTLSResolution{runtimeLang: runtimeLang}, nil
	}

	target, err := openOTelTargetProcess(pid)
	if err != nil {
		return otelTLSResolution{}, err
	}

	loaderKind := target.detectLoaderKind()
	modules, err := target.moduleImages()
	if err != nil {
		return otelTLSResolution{}, err
	}

	candidate, err := target.findTLSCandidate(otelTLSSymbolName)
	if err != nil {
		return otelTLSResolution{}, err
	}

	linkMapMode := target.hasDynamicLoader()
	if !linkMapMode && !candidate.mainExecutable {
		return otelTLSResolution{}, fmt.Errorf("no dynamic linker found and %q is not in the main executable", otelTLSSymbolName)
	}

	dtv := defaultOTelDTVLayout(loaderKind)
	res := otelTLSResolution{
		targetLoadBias:        candidate.loadBias,
		targetSymbolOffset:    candidate.symbolOffset,
		targetSymbolSize:      candidate.symbolSize,
		targetTLSMemsz:        candidate.tlsMemsz,
		rDebugRMapOffset:      rDebugRMapOffset,
		linkMapLAddrOffset:    linkMapLAddrOffset,
		linkMapLNextOffset:    linkMapLNextOffset,
		tcbDTVOffset:          dtv.tcbDTVOffset,
		dtvEntrySize:          dtv.dtvEntrySize,
		dtvEntryPointerOffset: dtv.dtvEntryPointerOffset,
		runtimeLang:           runtimeLang,
	}

	if !linkMapMode {
		res.mode = otelTLSModeStaticMain
		return res, nil
	}

	res.mode = otelTLSModeLinkMap
	res.dtDebugValueAddr, err = target.dtDebugValueAddr()
	if err != nil {
		return otelTLSResolution{}, err
	}

	if loaderKind == otelLoaderGlibc {
		res.linkMapLRealOffset = glibcLinkMapLRealOffset
		if threadDBLayout, ok := target.glibcThreadDBLayout(modules); ok {
			res.linkMapLTLSModIDOffset = threadDBLayout.linkMap.modIDOffset
			res.linkMapLTLSTPOffsetOffset = threadDBLayout.linkMap.tlsOffsetOffset
			res.tcbDTVOffset = threadDBLayout.dtv.tcbDTVOffset
			res.dtvEntrySize = threadDBLayout.dtv.dtvEntrySize
			res.dtvEntryPointerOffset = threadDBLayout.dtv.dtvEntryPointerOffset
			return res, nil
		}
	}

	res.reconstructModuleIDs = 1
	moduleSet, tlsModuleCount, err := buildOTelTLSModuleSet(modules, candidate)
	if err != nil {
		return otelTLSResolution{}, err
	}
	res.tlsModuleHashSeed = moduleSet.seed
	res.tlsModuleHashBits = moduleSet.bits
	res.tlsModuleCount = tlsModuleCount
	return res, nil
}

func openOTelTargetProcess(pid uint32) (*otelTargetProcess, error) {
	pidStr := strconv.FormatUint(uint64(pid), 10)
	exePath, err := os.Readlink(kernel.HostProc(pidStr, "exe"))
	if err != nil {
		return nil, fmt.Errorf("resolve /proc/%s/exe: %w", pidStr, err)
	}
	exePath = stripDeletedMapsSuffix(exePath)

	auxv, err := readOTelAuxv(pidStr)
	if err != nil {
		return nil, err
	}

	return &otelTargetProcess{
		pid:     pid,
		pidStr:  pidStr,
		exePath: exePath,
		auxv:    auxv,
	}, nil
}

func (p *otelTargetProcess) fsPath(path string) string {
	return kernel.HostProc(p.pidStr, "root", path)
}

func (p *otelTargetProcess) maps() ([]otelMapEntry, error) {
	mapsPath := kernel.HostProc(p.pidStr, "maps")
	file, err := os.Open(mapsPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mapsPath, err)
	}
	defer file.Close()

	var entries []otelMapEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if entry, ok := parseOTelMapsLine(scanner.Text()); ok {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", mapsPath, err)
	}
	return entries, nil
}

func (p *otelTargetProcess) groupedReadableFileMaps() (map[string][]otelMapEntry, []string, error) {
	entries, err := p.maps()
	if err != nil {
		return nil, nil, err
	}

	grouped := make(map[string][]otelMapEntry)
	var order []string
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.readableFileMapping() {
			continue
		}
		grouped[entry.path] = append(grouped[entry.path], entry)
		if _, ok := seen[entry.path]; !ok {
			seen[entry.path] = struct{}{}
			order = append(order, entry.path)
		}
	}
	return grouped, order, nil
}

func parseOTelMapsLine(line string) (otelMapEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return otelMapEntry{}, false
	}

	address := fields[0]
	dash := strings.IndexByte(address, '-')
	if dash <= 0 {
		return otelMapEntry{}, false
	}
	start, err := strconv.ParseUint(address[:dash], 16, 64)
	if err != nil {
		return otelMapEntry{}, false
	}
	end, err := strconv.ParseUint(address[dash+1:], 16, 64)
	if err != nil {
		return otelMapEntry{}, false
	}
	offset, err := strconv.ParseUint(fields[2], 16, 64)
	if err != nil {
		return otelMapEntry{}, false
	}

	path := ""
	if len(fields) > 5 {
		path = strings.Join(fields[5:], " ")
		path = stripDeletedMapsSuffix(path)
	}

	return otelMapEntry{
		start:  start,
		end:    end,
		offset: offset,
		perms:  fields[1],
		path:   path,
	}, true
}

func (e otelMapEntry) readableFileMapping() bool {
	return e.path != "" && e.path[0] == '/' && strings.HasPrefix(e.perms, "r")
}

func stripDeletedMapsSuffix(path string) string {
	return strings.TrimSuffix(path, " (deleted)")
}

func readOTelAuxv(pidStr string) (otelAuxvInfo, error) {
	auxvPath := kernel.HostProc(pidStr, "auxv")
	data, err := os.ReadFile(auxvPath)
	if err != nil {
		return otelAuxvInfo{}, fmt.Errorf("read %s: %w", auxvPath, err)
	}

	var info otelAuxvInfo
	for offset := 0; offset+16 <= len(data); offset += 16 {
		tag := binary.NativeEndian.Uint64(data[offset:])
		value := binary.NativeEndian.Uint64(data[offset+8:])
		if tag == 0 {
			break
		}
		if tag == 7 { // AT_BASE
			info.atBase = value
		}
	}
	return info, nil
}

func (p *otelTargetProcess) detectLoaderKind() otelLoaderKind {
	if p.hasMuslInterpreter() || p.hasMuslLoaderMapping() || p.hasStaticMuslMarkers() {
		return otelLoaderMusl
	}
	return otelLoaderGlibc
}

func (p *otelTargetProcess) hasDynamicLoader() bool {
	if p.auxv.atBase != 0 {
		return true
	}

	elfFile, err := openOTelELF(p.fsPath(p.exePath))
	if err != nil {
		return false
	}
	defer elfFile.Close()

	return elfInterpreter(elfFile) != ""
}

func (p *otelTargetProcess) hasMuslInterpreter() bool {
	elfFile, err := openOTelELF(p.fsPath(p.exePath))
	if err != nil {
		return false
	}
	defer elfFile.Close()

	return strings.Contains(elfInterpreter(elfFile), "/ld-musl-")
}

func (p *otelTargetProcess) hasMuslLoaderMapping() bool {
	entries, err := p.maps()
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.Contains(entry.path, "/ld-musl-") {
			return true
		}
	}
	return false
}

func (p *otelTargetProcess) hasStaticMuslMarkers() bool {
	elfFile, err := openOTelELF(p.fsPath(p.exePath))
	if err != nil {
		return false
	}
	defer elfFile.Close()

	_, mainTLS := symbolValueInDynsym(elfFile, "main_tls")
	_, builtinTLS := symbolValueInDynsym(elfFile, "builtin_tls")
	return mainTLS && builtinTLS
}

func (p *otelTargetProcess) moduleImages() ([]otelModuleImage, error) {
	grouped, order, err := p.groupedReadableFileMaps()
	if err != nil {
		return nil, err
	}

	pageSize := uint64(os.Getpagesize())
	modules := make([]otelModuleImage, 0, len(order))
	for _, path := range order {
		elfFile, err := openOTelELF(p.fsPath(path))
		if err != nil {
			continue
		}

		loadBias, err := elfLoadBias(elfFile, grouped[path], pageSize)
		if err != nil {
			elfFile.Close()
			continue
		}
		_, hasTLS := elfPTTLSMemsz(elfFile)
		elfFile.Close()

		modules = append(modules, otelModuleImage{
			path:     path,
			loadBias: loadBias,
			hasTLS:   hasTLS,
		})
	}
	return modules, nil
}

func (p *otelTargetProcess) findTLSCandidate(symbol string) (otelTLSCandidate, error) {
	grouped, order, err := p.groupedReadableFileMaps()
	if err != nil {
		return otelTLSCandidate{}, err
	}

	pageSize := uint64(os.Getpagesize())
	for _, path := range order {
		elfFile, err := openOTelELF(p.fsPath(path))
		if err != nil {
			continue
		}

		tlsMemsz, hasTLS := elfPTTLSMemsz(elfFile)
		if !hasTLS {
			elfFile.Close()
			continue
		}

		symbols := tlsSymbolsInDynsym(elfFile, symbol)
		if len(symbols) == 0 {
			elfFile.Close()
			continue
		}

		loadBias, err := elfLoadBias(elfFile, grouped[path], pageSize)
		if err != nil {
			elfFile.Close()
			continue
		}
		elfFile.Close()

		for _, tlsSymbol := range symbols {
			if !tlsSymbol.fitsInTLSSegment(tlsMemsz) {
				continue
			}

			return otelTLSCandidate{
				path:           path,
				loadBias:       loadBias,
				symbolOffset:   tlsSymbol.offset,
				symbolSize:     tlsSymbol.size,
				tlsMemsz:       tlsMemsz,
				mainExecutable: path == p.exePath,
			}, nil
		}
	}

	return otelTLSCandidate{}, fmt.Errorf("TLS symbol %q not found in currently mapped readable ELF objects", symbol)
}

func (p *otelTargetProcess) dtDebugValueAddr() (uint64, error) {
	grouped, _, err := p.groupedReadableFileMaps()
	if err != nil {
		return 0, err
	}

	maps, ok := grouped[p.exePath]
	if !ok {
		return 0, fmt.Errorf("main executable %q not found in /proc/%s/maps", p.exePath, p.pidStr)
	}

	elfFile, err := openOTelELF(p.fsPath(p.exePath))
	if err != nil {
		return 0, err
	}
	defer elfFile.Close()

	loadBias, err := elfLoadBias(elfFile, maps, uint64(os.Getpagesize()))
	if err != nil {
		return 0, err
	}
	vaddr, ok := elfDTDebugValueVaddr(elfFile)
	if !ok {
		return 0, fmt.Errorf("DT_DEBUG entry not found in main executable %q", p.exePath)
	}
	return loadBias + vaddr, nil
}

func (p *otelTargetProcess) glibcThreadDBLayout(modules []otelModuleImage) (otelGlibcThreadDBLayout, bool) {
	for _, module := range modules {
		if !strings.Contains(module.path, "libc.so") {
			continue
		}

		elfFile, err := openOTelELF(p.fsPath(module.path))
		if err != nil {
			continue
		}

		modID, okModID := elfDBDesc(elfFile, "_thread_db_link_map_l_tls_modid")
		tlsOffset, okTLSOffset := elfDBDesc(elfFile, "_thread_db_link_map_l_tls_offset")
		pthreadDTVP, okPthreadDTVP := elfDBDesc(elfFile, "_thread_db_pthread_dtvp")
		dtvDTV, okDTVDTV := elfDBDesc(elfFile, "_thread_db_dtv_dtv")
		pointerVal, okPointerVal := elfDBDesc(elfFile, "_thread_db_dtv_t_pointer_val")
		elfFile.Close()

		if !okModID || !okTLSOffset || !okPthreadDTVP || !okDTVDTV || !okPointerVal {
			continue
		}
		if dtvDTV.sizeBits == 0 || dtvDTV.sizeBits%8 != 0 {
			continue
		}

		return otelGlibcThreadDBLayout{
			linkMap: otelLinkMapTLSLayout{
				modIDOffset:     uint64(modID.offset),
				tlsOffsetOffset: uint64(tlsOffset.offset),
			},
			dtv: otelDTVLayout{
				tcbDTVOffset:          int64(pthreadDTVP.offset),
				dtvEntrySize:          uint64(dtvDTV.sizeBits / 8),
				dtvEntryPointerOffset: uint64(pointerVal.offset),
			},
		}, true
	}
	return otelGlibcThreadDBLayout{}, false
}

func defaultOTelDTVLayout(kind otelLoaderKind) otelDTVLayout {
	tcbDTVOffset := int64(0)
	switch runtime.GOARCH {
	case "amd64":
		tcbDTVOffset = 8
	case "arm64":
		if kind == otelLoaderMusl {
			tcbDTVOffset = -8
		}
	default:
		tcbDTVOffset = 0
	}

	entrySize := uint64(16)
	if kind == otelLoaderMusl {
		entrySize = 8
	}

	return otelDTVLayout{
		tcbDTVOffset:          tcbDTVOffset,
		dtvEntrySize:          entrySize,
		dtvEntryPointerOffset: 0,
	}
}

func buildOTelTLSModuleSet(modules []otelModuleImage, candidate otelTLSCandidate) (otelTLSModuleSet, uint32, error) {
	var tlsModuleCount uint32
	targetPresent := false
	for _, module := range modules {
		if !module.hasTLS {
			continue
		}
		if tlsModuleCount == otelTLSMaxModules {
			return otelTLSModuleSet{}, 0, fmt.Errorf("too many TLS modules; max supported is %d", otelTLSMaxModules)
		}
		tlsModuleCount++
		if module.loadBias == candidate.loadBias {
			targetPresent = true
		}
	}
	if !targetPresent {
		return otelTLSModuleSet{}, 0, fmt.Errorf("target TLS module %q was not present in file-derived PT_TLS module set", candidate.path)
	}

	for seed := uint64(0); seed < otelTLSMaxSeedSearch; seed++ {
		set := otelTLSModuleSet{seed: seed}
		for _, module := range modules {
			if module.hasTLS {
				set.set(otelTLSHashSlot(module.loadBias, seed))
			}
		}
		if set.rejectsNonTLSModules(modules) {
			return set, tlsModuleCount, nil
		}
	}

	return otelTLSModuleSet{}, 0, fmt.Errorf("could not build TLS module membership table")
}

func otelTLSHashSlot(loadBias uint64, seed uint64) uint32 {
	hash := loadBias >> 12
	hash ^= seed
	hash ^= hash >> 33
	hash *= otelTLSHashMul1
	hash ^= hash >> 33
	hash *= otelTLSHashMul2
	hash ^= hash >> 33
	return uint32(hash & (otelTLSHashSlots - 1))
}

func (s *otelTLSModuleSet) set(slot uint32) {
	s.bits[slot>>6] |= uint64(1) << (slot & 63)
}

func (s otelTLSModuleSet) test(slot uint32) bool {
	return (s.bits[slot>>6] & (uint64(1) << (slot & 63))) != 0
}

func (s otelTLSModuleSet) rejectsNonTLSModules(modules []otelModuleImage) bool {
	for _, module := range modules {
		if !module.hasTLS && s.test(otelTLSHashSlot(module.loadBias, s.seed)) {
			return false
		}
	}
	return true
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

func elfPTTLSMemsz(elfFile *safeelf.File) (uint64, bool) {
	for _, prog := range elfFile.Progs {
		if prog.Type == elf.PT_TLS {
			return prog.Memsz, true
		}
	}
	return 0, false
}

func elfLoadBias(elfFile *safeelf.File, maps []otelMapEntry, pageSize uint64) (uint64, error) {
	if elfFile.Type == elf.ET_EXEC {
		return 0, nil
	}
	if len(maps) == 0 {
		return 0, fmt.Errorf("no maps entries for ELF")
	}

	anchor := maps[0]
	for _, entry := range maps[1:] {
		if entry.start < anchor.start {
			anchor = entry
		}
	}

	for _, prog := range elfFile.Progs {
		if prog.Type != elf.PT_LOAD {
			continue
		}

		phdrOffset := alignDown(prog.Off, pageSize)
		phdrVaddr := alignDown(prog.Vaddr, pageSize)
		if anchor.offset == phdrOffset && anchor.start >= phdrVaddr {
			return anchor.start - phdrVaddr, nil
		}
	}

	return 0, fmt.Errorf("could not compute load bias")
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

func elfDTDebugValueVaddr(elfFile *safeelf.File) (uint64, bool) {
	for _, prog := range elfFile.Progs {
		if prog.Type != elf.PT_DYNAMIC || prog.Filesz < 16 {
			continue
		}

		data, err := readELFProgBytes(prog, 0, int(prog.Filesz))
		if err != nil {
			continue
		}

		for off := 0; off+16 <= len(data); off += 16 {
			tag := elfFile.ByteOrder.Uint64(data[off:])
			if tag == uint64(elf.DT_DEBUG) {
				return prog.Vaddr + uint64(off) + 8, true
			}
			if tag == uint64(elf.DT_NULL) {
				break
			}
		}
	}
	return 0, false
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

func symbolValueInDynsym(elfFile *safeelf.File, name string) (uint64, bool) {
	syms, err := elfFile.DynamicSymbols()
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

func tlsSymbolsInDynsym(elfFile *safeelf.File, name string) []otelTLSSymbol {
	syms, err := elfFile.DynamicSymbols()
	if err != nil {
		return nil
	}

	var out []otelTLSSymbol
	for _, sym := range syms {
		if sym.Name != name || sym.Section == elf.SHN_UNDEF || safeelf.ST_TYPE(sym.Info) != elf.STT_TLS {
			continue
		}
		out = append(out, otelTLSSymbol{
			name:   sym.Name,
			offset: sym.Value,
			size:   sym.Size,
		})
	}
	return out
}

func (s otelTLSSymbol) fitsInTLSSegment(tlsMemsz uint64) bool {
	return s.offset <= tlsMemsz && s.size <= tlsMemsz-s.offset
}

type otelDBDesc struct {
	sizeBits uint32
	nelem    uint32
	offset   uint32
}

func elfDBDesc(elfFile *safeelf.File, name string) (otelDBDesc, bool) {
	value, ok := symbolValueInDynsym(elfFile, name)
	if !ok {
		return otelDBDesc{}, false
	}

	data, err := readELFLoadVaddr(elfFile, value, 12)
	if err != nil {
		return otelDBDesc{}, false
	}

	return otelDBDesc{
		sizeBits: elfFile.ByteOrder.Uint32(data[0:4]),
		nelem:    elfFile.ByteOrder.Uint32(data[4:8]),
		offset:   elfFile.ByteOrder.Uint32(data[8:12]),
	}, true
}

func readELFLoadVaddr(elfFile *safeelf.File, vaddr uint64, size int) ([]byte, error) {
	if size < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	for _, prog := range elfFile.Progs {
		if prog.Type != elf.PT_LOAD || vaddr < prog.Vaddr {
			continue
		}
		delta := vaddr - prog.Vaddr
		if delta > prog.Filesz || uint64(size) > prog.Filesz-delta {
			continue
		}
		return readELFProgBytes(prog, delta, size)
	}

	return nil, fmt.Errorf("vaddr %#x not found in PT_LOAD", vaddr)
}

// serializeOTelTLSValue serializes otelTLSResolution as struct otel_tls_t.
func serializeOTelTLSValue(res otelTLSResolution) []byte {
	buf := make([]byte, otelTLSValueSize)
	binary.NativeEndian.PutUint64(buf[0:8], res.dtDebugValueAddr)
	binary.NativeEndian.PutUint64(buf[8:16], res.targetLoadBias)
	binary.NativeEndian.PutUint64(buf[16:24], res.targetSymbolOffset)
	binary.NativeEndian.PutUint64(buf[24:32], res.targetSymbolSize)
	binary.NativeEndian.PutUint64(buf[32:40], res.targetTLSMemsz)
	binary.NativeEndian.PutUint64(buf[40:48], res.rDebugRMapOffset)
	binary.NativeEndian.PutUint64(buf[48:56], res.linkMapLAddrOffset)
	binary.NativeEndian.PutUint64(buf[56:64], res.linkMapLNextOffset)
	binary.NativeEndian.PutUint64(buf[64:72], res.linkMapLRealOffset)
	binary.NativeEndian.PutUint64(buf[72:80], res.linkMapLTLSModIDOffset)
	binary.NativeEndian.PutUint64(buf[80:88], res.linkMapLTLSTPOffsetOffset)
	binary.NativeEndian.PutUint64(buf[88:96], uint64(res.tcbDTVOffset))
	binary.NativeEndian.PutUint64(buf[96:104], res.dtvEntrySize)
	binary.NativeEndian.PutUint64(buf[104:112], res.dtvEntryPointerOffset)
	binary.NativeEndian.PutUint64(buf[112:120], res.tlsModuleHashSeed)

	offset := 120
	for _, word := range res.tlsModuleHashBits {
		binary.NativeEndian.PutUint64(buf[offset:offset+8], word)
		offset += 8
	}

	// resolved_mod_id, resolved_static_tls_offset, resolved_read_error,
	// resolved, status, and _pad are intentionally zeroed: eBPF fills them
	// after reading the target's live loader state.
	binary.NativeEndian.PutUint32(buf[offset+20:offset+24], res.mode)
	binary.NativeEndian.PutUint32(buf[offset+24:offset+28], res.reconstructModuleIDs)
	binary.NativeEndian.PutUint32(buf[offset+28:offset+32], res.tlsModuleCount)
	binary.NativeEndian.PutUint32(buf[offset+32:offset+36], res.runtimeLang)
	return buf
}
