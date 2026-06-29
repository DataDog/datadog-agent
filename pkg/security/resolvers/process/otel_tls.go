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

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
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
	//
	// WARNING: only the glibc layout is exercised by the test suite. musl's
	// debug link_map is assumed to share the same public field offsets
	// (l_addr at 0, l_next at 24) but this has not been validated against a
	// real musl process, and musl link-map mode additionally always takes the
	// reconstructModuleIDs fallback (see resolveOTelTLS). Treat musl support as
	// best-effort until a musl fixture is added to TestOTelSpan.
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

// otelMappedELF holds the metadata derived from a single open of one mapped
// ELF object. It is computed once per resolveOTelTLS call by mappedELFs(), so
// that each mapped file is read and parsed exactly once.
type otelMappedELF struct {
	path     string
	loadBias uint64
	hasTLS   bool
	tlsMemsz uint64

	// otelTLSSyms holds STT_TLS symbols named otelTLSSymbolName, if any.
	otelTLSSyms []otelTLSSymbol

	// The following fields are only populated for the main executable.
	isMainExe    bool
	interpreter  string
	dtDebugVaddr uint64
	hasDTDebug   bool
	mainTLS      bool // musl static marker
	builtinTLS   bool // musl static marker

	// threadDB is populated for libc.so objects exposing thread_db descriptors.
	threadDB    otelGlibcThreadDBLayout
	hasThreadDB bool
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

	// Memoized state, each computed lazily exactly once per process so that
	// /proc/<pid>/maps and each mapped ELF are read only a single time.
	mapsGrouped map[string][]procfs.MapsEntry
	mapsOrder   []string
	mapsErr     error
	mapsDone    bool

	mappedELFs    []otelMappedELF
	mappedELFsErr error
	mappedELFDone bool
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

	modules, err := target.loadMappedELFs()
	if err != nil {
		return otelTLSResolution{}, err
	}

	loaderKind := target.detectLoaderKind(modules)

	candidate, ok := findOTelTLSCandidate(modules)
	if !ok {
		return otelTLSResolution{}, fmt.Errorf("TLS symbol %q not found in currently mapped readable ELF objects", otelTLSSymbolName)
	}

	linkMapMode := target.hasDynamicLoader(modules)
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
	res.dtDebugValueAddr, err = dtDebugValueAddr(modules)
	if err != nil {
		return otelTLSResolution{}, err
	}

	if loaderKind == otelLoaderGlibc {
		res.linkMapLRealOffset = glibcLinkMapLRealOffset
		if threadDBLayout, ok := glibcThreadDBLayout(modules); ok {
			res.linkMapLTLSModIDOffset = threadDBLayout.linkMap.modIDOffset
			res.linkMapLTLSTPOffsetOffset = threadDBLayout.linkMap.tlsOffsetOffset
			res.tcbDTVOffset = threadDBLayout.dtv.tcbDTVOffset
			res.dtvEntrySize = threadDBLayout.dtv.dtvEntrySize
			res.dtvEntryPointerOffset = threadDBLayout.dtv.dtvEntryPointerOffset
			return res, nil
		}
	}

	// Fallback path: reconstruct the runtime TLS module ID in eBPF by counting
	// PT_TLS modules in loader order rather than reading l_tls_modid directly.
	// glibc reaches here only when its thread_db descriptors are missing, but
	// musl ALWAYS reaches here (no thread_db). This reconstruction can diverge
	// from the real module ID when TLS modules are assigned non-sequentially
	// (e.g. after dlclose reuse), and the musl link-map walk it depends on is
	// not covered by tests. See the loader-offset constants above.
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

// loadMappedELFs opens each readable file-backed mapping of the target process
// exactly once and extracts every piece of metadata the resolver needs. The
// result is memoized so repeated callers share a single /proc/<pid>/maps scan
// and a single open+parse per mapped ELF.
func (p *otelTargetProcess) loadMappedELFs() ([]otelMappedELF, error) {
	if p.mappedELFDone {
		return p.mappedELFs, p.mappedELFsErr
	}
	p.mappedELFDone = true

	grouped, order, err := p.groupedReadableFileMaps()
	if err != nil {
		p.mappedELFsErr = err
		return nil, err
	}

	pageSize := uint64(os.Getpagesize())
	modules := make([]otelMappedELF, 0, len(order))
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

		tlsMemsz, hasTLS := elfPTTLSMemsz(elfFile)
		module := otelMappedELF{
			path:      path,
			loadBias:  loadBias,
			hasTLS:    hasTLS,
			tlsMemsz:  tlsMemsz,
			isMainExe: path == p.exePath,
		}
		if hasTLS {
			module.otelTLSSyms = tlsSymbolsInDynsym(elfFile, otelTLSSymbolName)
		}
		if module.isMainExe {
			module.interpreter = elfInterpreter(elfFile)
			module.dtDebugVaddr, module.hasDTDebug = elfDTDebugValueVaddr(elfFile)
			_, module.mainTLS = symbolValueInDynsym(elfFile, "main_tls")
			_, module.builtinTLS = symbolValueInDynsym(elfFile, "builtin_tls")
		}
		if strings.Contains(path, "libc.so") {
			module.threadDB, module.hasThreadDB = readGlibcThreadDBLayout(elfFile)
		}
		elfFile.Close()

		modules = append(modules, module)
	}

	p.mappedELFs = modules
	return modules, nil
}

func (p *otelTargetProcess) detectLoaderKind(modules []otelMappedELF) otelLoaderKind {
	// A mapped musl loader is conclusive on its own. Scan the maps order
	// directly so detection does not depend on the loader ELF parsing cleanly.
	for _, path := range p.mapsOrder {
		if strings.Contains(path, "/ld-musl-") {
			return otelLoaderMusl
		}
	}
	// Otherwise rely on main-executable markers: a musl PT_INTERP, or the
	// static-musl main_tls/builtin_tls symbol pair.
	for _, module := range modules {
		if !module.isMainExe {
			continue
		}
		if strings.Contains(module.interpreter, "/ld-musl-") {
			return otelLoaderMusl
		}
		if module.mainTLS && module.builtinTLS {
			return otelLoaderMusl
		}
	}
	return otelLoaderGlibc
}

func (p *otelTargetProcess) hasDynamicLoader(modules []otelMappedELF) bool {
	if p.auxv.atBase != 0 {
		return true
	}
	for _, module := range modules {
		if module.isMainExe {
			return module.interpreter != ""
		}
	}
	return false
}

func findOTelTLSCandidate(modules []otelMappedELF) (otelTLSCandidate, bool) {
	for _, module := range modules {
		if !module.hasTLS {
			continue
		}
		for _, tlsSymbol := range module.otelTLSSyms {
			if !tlsSymbol.fitsInTLSSegment(module.tlsMemsz) {
				continue
			}
			return otelTLSCandidate{
				path:           module.path,
				loadBias:       module.loadBias,
				symbolOffset:   tlsSymbol.offset,
				symbolSize:     tlsSymbol.size,
				tlsMemsz:       module.tlsMemsz,
				mainExecutable: module.isMainExe,
			}, true
		}
	}
	return otelTLSCandidate{}, false
}

func dtDebugValueAddr(modules []otelMappedELF) (uint64, error) {
	for _, module := range modules {
		if !module.isMainExe {
			continue
		}
		if !module.hasDTDebug {
			return 0, fmt.Errorf("DT_DEBUG entry not found in main executable %q", module.path)
		}
		return module.loadBias + module.dtDebugVaddr, nil
	}
	return 0, fmt.Errorf("main executable not found among mapped readable ELF objects")
}

func glibcThreadDBLayout(modules []otelMappedELF) (otelGlibcThreadDBLayout, bool) {
	for _, module := range modules {
		if module.hasThreadDB {
			return module.threadDB, true
		}
	}
	return otelGlibcThreadDBLayout{}, false
}

// readGlibcThreadDBLayout reads the glibc thread_db descriptor symbols from an
// already-open libc ELF object.
func readGlibcThreadDBLayout(elfFile *safeelf.File) (otelGlibcThreadDBLayout, bool) {
	modID, okModID := elfDBDesc(elfFile, "_thread_db_link_map_l_tls_modid")
	tlsOffset, okTLSOffset := elfDBDesc(elfFile, "_thread_db_link_map_l_tls_offset")
	pthreadDTVP, okPthreadDTVP := elfDBDesc(elfFile, "_thread_db_pthread_dtvp")
	dtvDTV, okDTVDTV := elfDBDesc(elfFile, "_thread_db_dtv_dtv")
	pointerVal, okPointerVal := elfDBDesc(elfFile, "_thread_db_dtv_t_pointer_val")

	if !okModID || !okTLSOffset || !okPthreadDTVP || !okDTVDTV || !okPointerVal {
		return otelGlibcThreadDBLayout{}, false
	}
	if dtvDTV.sizeBits == 0 || dtvDTV.sizeBits%8 != 0 {
		return otelGlibcThreadDBLayout{}, false
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

func buildOTelTLSModuleSet(modules []otelMappedELF, candidate otelTLSCandidate) (otelTLSModuleSet, uint32, error) {
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

	return otelTLSModuleSet{}, 0, fmt.Errorf(
		"could not build collision-free TLS module membership table after %d seeds (%d TLS / %d total modules)",
		otelTLSMaxSeedSearch, tlsModuleCount, len(modules))
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

func (s otelTLSModuleSet) rejectsNonTLSModules(modules []otelMappedELF) bool {
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

func elfLoadBias(elfFile *safeelf.File, maps []procfs.MapsEntry, pageSize uint64) (uint64, error) {
	if elfFile.Type == elf.ET_EXEC {
		return 0, nil
	}
	if len(maps) == 0 {
		return 0, fmt.Errorf("no maps entries for ELF")
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
