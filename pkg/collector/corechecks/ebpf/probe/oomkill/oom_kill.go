// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/oom-kill-kern.c pkg/ebpf/bytecode/build/runtime/oom-kill.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/oom-kill.c pkg/ebpf/bytecode/runtime/oom-kill.go runtime

// Package oomkill is the system-probe side of the OOM Kill check
package oomkill

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"

	bpflib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	oomMapName    = "oom_stats"
	oomKprobeName = "kprobe__oom_kill_process"
	oomFentryName = "fentry__oom_kill_process"
	oomHookName   = "oom_kill_process"
	// oomLinkName names the link pin, whichever program the link attaches
	oomLinkName = "oom_kill_process"
	moduleName  = "oom_kill"
	// oomStatsVersionField is the member of struct oom_stats whose type name, e.g. oom_stats_v1,
	// is bumped whenever the layout of the struct changes
	oomStatsVersionField = "oom_stats_version"
	// emitCgroupMemoryStatsConst is the LOAD_CONSTANT of the eBPF program set from
	// oom_kill.emit_cgroup_memory_stats
	emitCgroupMemoryStatsConst = "emit_cgroup_memory_stats"
	// memcgFlushBeginName and memcgFlushEndName count the memcg stats flush steps into
	// memcg_flush_seq, which the oom-kill programs check the cgroup memory stats against
	memcgFlushBeginName = "fentry__mem_cgroup_css_rstat_flush"
	memcgFlushEndName   = "fexit__mem_cgroup_css_rstat_flush"
	memcgFlushHookName  = "mem_cgroup_css_rstat_flush"
	memcgFlushSeqName   = "memcg_flush_seq"
	// memcgFlushBeginLinkName and memcgFlushEndLinkName name the link pins of the memcg flush
	// programs
	memcgFlushBeginLinkName = "memcg_flush_begin"
	memcgFlushEndLinkName   = "memcg_flush_end"
)

var supportsFentry = features.SupportsFentry

var (
	bpffsRoot = "/sys/fs/bpf"
	pinDir    = filepath.Join(bpffsRoot, "datadog-agent", "oom_kill")
)

// Probe is the eBPF side of the OOM Kill check
type Probe struct {
	coll *bpflib.Collection
	// progName is the program attached to oom_kill_process: oomFentryName or oomKprobeName
	progName string
	link     link.Link
	oomMap   *maps.GenericMap[statsKey, oomStats]
	// flushLinks attach the memcg flush programs, when the cgroup memory stats are collected. They
	// are keyed by their pin name: memcgFlushBeginLinkName or memcgFlushEndLinkName
	flushLinks map[string]link.Link
	// emitStats is true if the oom-kill program collects the cgroup memory stats
	emitStats bool
	// pinned is true if this probe owns the bpffs pins of the link and map
	pinned bool
}

// NewProbe creates a [Probe].
//
// If cfg.PersistMonitoring is true, the probe is pinned to bpffs so that it keeps recording OOM kills
// after system-probe exits without a graceful shutdown. Whatever its value, a probe left pinned by
// a previous system-probe is taken over: the OOM kills it recorded are reported by the next
// [Probe.GetAndFlush], and when cfg.PersistMonitoring is false its pins are removed.
func NewProbe(cfg *Config) (*Probe, error) {
	if cfg.EnableCORE {
		probe, err := loadOOMKillCOREProbe(cfg)
		if err == nil {
			return probe, nil
		}

		if !cfg.AllowRuntimeCompiledFallback {
			return nil, fmt.Errorf("error loading CO-RE oom-kill probe: %s. set system_probe_config.allow_runtime_compiled_fallback to true to allow fallback to runtime compilation", err)
		}
		log.Warnf("error loading CO-RE oom-kill probe: %s. falling back to runtime compiled probe", err)
	}

	return loadOOMKillRuntimeCompiledProbe(cfg)
}

func loadOOMKillCOREProbe(cfg *Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %s", err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but oom-kill probe requires a kernel version of at least 4.9.0", kv)
	}

	var probe *Probe
	err = ebpf.LoadCOREAsset("oom-kill.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startOOMKillProbe(buf, opts.VerifierOptions, cfg)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of oom-kill probe")
	return probe, nil
}

func loadOOMKillRuntimeCompiledProbe(cfg *Config) (*Probe, error) {
	buf, err := runtime.OomKill.Compile(&cfg.Config, getCFlags(&cfg.Config))
	if err != nil {
		return nil, err
	}
	defer buf.Close()

	return startOOMKillProbe(buf, bpflib.CollectionOptions{}, cfg)
}

func getCFlags(config *ebpf.Config) []string {
	cflags := []string{"-g"}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

func startOOMKillProbe(buf bytecode.AssetReader, collOpts bpflib.CollectionOptions, cfg *Config) (*Probe, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock rlimit: %w", err)
	}

	spec, err := bpflib.LoadCollectionSpecFromReader(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to load collection spec: %w", err)
	}
	mapSpec, ok := spec.Maps[oomMapName]
	if !ok {
		return nil, fmt.Errorf("map '%s' not found", oomMapName)
	}

	progSpec, err := selectProgram(spec)
	if err != nil {
		return nil, err
	}

	// The cgroup memory stats are checked against the memcg stats flushes, which the memcg flush
	// programs count. Without them the check would always pass, so the stats are not collected.
	emitStats := uint64(0)
	if cfg.EnableCgroupMemoryStats {
		if addrs, err := cgroupMemoryStatsKernelAddresses(spec, progSpec); err != nil {
			log.Warnf("cgroup memory stats disabled: %s", err)
		} else {
			emitStats = 1
			for sym, name := range kernelVariableConsts {
				rewriteConstant(spec, name, addrs[sym])
			}
		}
	}
	if emitStats == 0 {
		delete(spec.Programs, memcgFlushBeginName)
		delete(spec.Programs, memcgFlushEndName)
	}
	rewriteConstant(spec, emitCgroupMemoryStatsConst, emitStats)

	// Pins left by a previous system-probe are taken over and cleaned up below even when this one
	// does not pin, so that they are not left running forever.
	pinning := pinningFeaturesSupported(progSpec) && cfg.PersistMonitoring

	if !pinning && cfg.PersistMonitoring {
		log.Warnf("pinning unsupported. cannot respect the 'oom_kill.persist_monitoring' configuration")
	}

	// Reuse the map of the previous probe, so that the OOM kills recorded while no system-probe
	// was running are reported by the next check run.
	if m := reusePinnedMap(oomMapName, func(m *bpflib.Map) error { return compatibleMap(mapSpec, m) }, &collOpts); m != nil {
		// close the reference to the pinned map this program takes so it can be fully reaped
		// after the pin is removed
		defer m.Close()
	}

	// Reuse the counter of the previous probe, so that the pinned memcg flush and oom-kill programs
	// share it with the ones of this probe and can be taken over. When the cgroup memory stats are
	// not collected, it is removed with the other pins of the previous probe.
	if emitStats == 1 {
		seqSpec := spec.Maps[memcgFlushSeqName]
		if m := reusePinnedMap(memcgFlushSeqName, seqSpec.Compatible, &collOpts); m != nil {
			defer m.Close()
		}
	}

	coll, err := bpflib.NewCollectionWithOptions(spec, collOpts)
	if err != nil {
		var ve *bpflib.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("verifier error loading oom-kill collection: %w\n%+v", err, ve)
		}
		return nil, fmt.Errorf("failed to load oom-kill collection: %w", err)
	}

	p := &Probe{coll: coll, progName: progSpec.Name, emitStats: emitStats == 1}
	if p.oomMap, err = maps.Map[statsKey, oomStats](coll.Maps[oomMapName]); err != nil {
		coll.Close()
		return nil, fmt.Errorf("failed to get map '%s': %w", oomMapName, err)
	}

	if err := p.attach(); err != nil {
		coll.Close()
		return nil, err
	}

	removePins()

	if pinning {
		if err := p.pin(); err != nil {
			log.Errorf("failed to pin ebpf resources: %v. Continuing unpinned.", err)
		}
	}

	ebpf.AddNameMappingsCollection(coll, moduleName)
	return p, nil
}

// reusePinnedMap loads the map pinned for name and, if compatible accepts it, makes the collection
// use it in place of creating the map. Otherwise the pin is removed. The returned map, if any, must
// be closed once the collection is loaded.
func reusePinnedMap(name string, compatible func(*bpflib.Map) error, collOpts *bpflib.CollectionOptions) *bpflib.Map {
	m, err := bpflib.LoadPinnedMap(mapPinPath(name), nil)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warnf("failed to load pinned map '%s', removing it: %s", name, err)
			removePin(mapPinPath(name))
		}
		return nil
	}

	if err := compatible(m); err != nil {
		log.Warnf("pinned map '%s' is incompatible with the current probe, dropping its content: %s", name, err)
		_ = m.Close()
		removePin(mapPinPath(name))
		return nil
	}

	if collOpts.MapReplacements == nil {
		collOpts.MapReplacements = make(map[string]*bpflib.Map)
	}
	collOpts.MapReplacements[name] = m
	return m
}

// compatibleMap checks that the pinned map can be reused by the current probe: on top of the
// attributes checked by MapSpec.Compatible, its values must have the same version marker, since a
// change of layout keeping the same size would otherwise be decoded wrongly.
func compatibleMap(mapSpec *bpflib.MapSpec, m *bpflib.Map) error {
	if err := mapSpec.Compatible(m); err != nil {
		return err
	}

	want, ok := btf.As[*btf.Struct](mapSpec.Value)
	if !ok {
		return fmt.Errorf("value of map '%s' is not a struct", oomMapName)
	}
	wantVersion, err := oomStatsVersion(want)
	if err != nil {
		return err
	}

	h, err := m.Handle()
	if err != nil {
		return fmt.Errorf("get BTF of pinned map: %w", err)
	}
	defer h.Close()
	pinnedSpec, err := h.Spec(nil)
	if err != nil {
		return fmt.Errorf("parse BTF of pinned map: %w", err)
	}
	var got *btf.Struct
	if err := pinnedSpec.TypeByName(want.Name, &got); err != nil {
		return fmt.Errorf("find struct %s in BTF of pinned map: %w", want.Name, err)
	}
	gotVersion, err := oomStatsVersion(got)
	if err != nil {
		return fmt.Errorf("pinned map: %w", err)
	}

	if gotVersion != wantVersion {
		return fmt.Errorf("pinned map has version %s, expected %s", gotVersion, wantVersion)
	}
	return nil
}

// oomStatsVersion returns the name of the type of the version marker of struct oom_stats
func oomStatsVersion(s *btf.Struct) (string, error) {
	for _, m := range s.Members {
		if m.Name != oomStatsVersionField {
			continue
		}
		td, ok := m.Type.(*btf.Typedef)
		if !ok {
			return "", fmt.Errorf("field %s of struct %s is %v, not a typedef", oomStatsVersionField, s.Name, m.Type)
		}
		return td.Name, nil
	}
	return "", fmt.Errorf("struct %s has no field %s", s.Name, oomStatsVersionField)
}

// pin pins the probe to bpffs, all or nothing.
//
// memcg_flush_seq and the memcg flush links are pinned before the oom-kill link, so that a crash
// while pinning never leaves a pinned oom-kill program checking the cgroup memory stats against a
// counter that nothing updates.
func (k *Probe) pin() error {
	if err := os.MkdirAll(pinDir, 0o700); err != nil {
		return fmt.Errorf("create pin directory %s: %w", pinDir, err)
	}

	var pins []string
	removeAddedPins := func() {
		for i := len(pins) - 1; i >= 0; i-- {
			removePin(pins[i])
		}
	}

	if len(k.flushLinks) > 0 {
		path := mapPinPath(memcgFlushSeqName)
		if err := k.coll.Maps[memcgFlushSeqName].Pin(path); err != nil {
			return fmt.Errorf("pin map '%s': %w", memcgFlushSeqName, err)
		}
		pins = append(pins, path)

		for _, name := range []string{memcgFlushBeginLinkName, memcgFlushEndLinkName} {
			path := linkPinPath(name)
			if err := pinLink(k.flushLinks[name], path); err != nil {
				removeAddedPins()
				return fmt.Errorf("pin link '%s': %w", name, err)
			}
			pins = append(pins, path)
		}
	}

	path := mapPinPath(oomMapName)
	if err := k.oomMap.Map().Pin(path); err != nil {
		removeAddedPins()
		return fmt.Errorf("pin map: %w", err)
	}
	pins = append(pins, path)

	if err := pinLink(k.link, linkPinPath(oomLinkName)); err != nil {
		removeAddedPins()
		return fmt.Errorf("pin link: %w", err)
	}

	k.pinned = true

	return nil
}

func pinLink(l link.Link, path string) error {
	// A link taken over from a pin still records that path, which makes Pin to the same path a
	// no-op even though removePins deleted the file. Clear it so that the link is pinned again.
	_ = l.Unpin()
	return l.Pin(path)
}

func (k *Probe) unpin() {
	ownsPin := func() bool {
		pinned, err := link.LoadPinnedLink(linkPinPath(oomLinkName), nil)
		if err != nil {
			return false
		}
		defer pinned.Close()
		pinnedInfo, err := pinned.Info()
		if err != nil {
			return false
		}
		ownInfo, err := k.link.Info()
		if err != nil {
			return false
		}
		return pinnedInfo.ID == ownInfo.ID
	}

	if !ownsPin() {
		log.Warnf("owned oom-kill pins not discovered")
		return
	}
	removePins()
}

func (k *Probe) attach() error {
	// The memcg flush programs are attached first: the oom-kill program, pinned or not, must not
	// check the cgroup memory stats against a counter that nothing updates.
	if err := k.attachMemcgFlushPrograms(); err != nil {
		return err
	}

	pinnedLink := loadPinnedLink(oomLinkName)

	prog := k.coll.Programs[k.progName]
	// The oom-kill program also reads memcg_flush_seq when it collects the cgroup memory stats, so
	// a pinned one reading the counter of a previous probe, which nothing updates anymore, is not
	// taken over.
	usedMaps := []*bpflib.Map{k.oomMap.Map()}
	if len(k.flushLinks) > 0 {
		usedMaps = append(usedMaps, k.coll.Maps[memcgFlushSeqName])
	}
	if pinnedProg := samePinnedProgram(pinnedLink, prog, usedMaps...); pinnedProg != nil {
		log.Infof("taking ownership of pinned oom-kill probe")
		_ = prog.Close()
		k.coll.Programs[k.progName] = pinnedProg
		k.link = pinnedLink
		return nil
	}
	// give up ownership over pinned link so it can be fully released
	if pinnedLink != nil {
		_ = pinnedLink.Close()
	}

	var l link.Link
	var err error
	if prog.Type() == bpflib.Tracing {
		l, err = link.AttachTracing(link.TracingOptions{Program: prog})
	} else {
		l, err = link.Kprobe(oomHookName, prog, &link.KprobeOptions{
			TraceFSPrefix: "ddoom",
		})
	}
	if err != nil {
		k.closeMemcgFlushLinks()
		return fmt.Errorf("failed to attach %s to %s: %w", k.progName, oomHookName, err)
	}
	k.link = l

	return nil
}

// cgroupMemoryStatsKernelAddresses returns the address of the kernel variables of
// kernelVariableConsts if the cgroup memory stats can be collected.
func cgroupMemoryStatsKernelAddresses(spec *bpflib.CollectionSpec, progSpec *bpflib.ProgramSpec) (map[string]uint64, error) {
	if progSpec.Type != bpflib.Tracing {
		return nil, fmt.Errorf("%s is used instead of %s", progSpec.Name, oomFentryName)
	}
	if err := cgroupMemoryStatsSupported(); err != nil {
		return nil, err
	}
	if err := memcgFlushTrackingSupported(spec); err != nil {
		return nil, fmt.Errorf("memcg stats flushes cannot be tracked: %w", err)
	}
	return kernelVariableAddresses()
}

// cgroupMemoryStatsSupported returns nil if the cgroup memory stats can be collected on the running
// kernel.
func cgroupMemoryStatsSupported() error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return fmt.Errorf("detect kernel version: %w", err)
	}
	if kv < kernel.VersionCode(6, 8, 0) {
		return fmt.Errorf("kernel %s is older than 6.8", kv)
	}
	return nil
}

// kernelVariableConsts maps the kernel variables the cgroup memory stats read to the LOAD_CONSTANT
// of the eBPF program set to their address. Kernel BTF only describes the per-cpu variables, so
// they cannot be typed ksyms.
var kernelVariableConsts = map[string]string{
	"__per_cpu_offset":       "per_cpu_offset_addr",
	"nr_cpu_ids":             "nr_cpu_ids_addr",
	"mem_cgroup_stats_index": "mem_cgroup_stats_index_addr",
}

// kernelVariableAddresses returns the address of the kernel variables of kernelVariableConsts. They
// are read with a kallsyms bpf iterator, which unlike /proc/kallsyms is not subject to
// kptr_restrict.
func kernelVariableAddresses() (map[string]uint64, error) {
	syms := make([]string, 0, len(kernelVariableConsts))
	for sym := range kernelVariableConsts {
		syms = append(syms, sym)
	}

	addrs, err := ebpf.GetKernelSymbolsAddressesWithKallsymsIterator(syms...)
	if addrs == nil {
		return nil, fmt.Errorf("read kernel symbol addresses: %w", err)
	}

	required := []string{"__per_cpu_offset", "nr_cpu_ids"}
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("detect kernel version: %w", err)
	}
	// mem_cgroup_stats_index only exists, and is only needed, from 6.10.
	if kv >= kernel.VersionCode(6, 10, 0) {
		required = append(required, "mem_cgroup_stats_index")
	}
	for _, sym := range required {
		if addrs[sym] == 0 {
			return nil, fmt.Errorf("address of kernel symbol %s not found", sym)
		}
	}
	return addrs, nil
}

// memcgFlushTrackingSupported returns nil if the memcg flush programs of spec can be loaded and
// attached on the running kernel.
func memcgFlushTrackingSupported(spec *bpflib.CollectionSpec) error {
	for _, name := range []string{memcgFlushBeginName, memcgFlushEndName} {
		if _, ok := spec.Programs[name]; !ok {
			return fmt.Errorf("program '%s' not found", name)
		}
	}
	if _, ok := spec.Maps[memcgFlushSeqName]; !ok {
		return fmt.Errorf("map '%s' not found", memcgFlushSeqName)
	}

	if err := features.SupportsFentry(memcgFlushHookName); err != nil {
		return fmt.Errorf("fentry on %s: %w", memcgFlushHookName, err)
	}
	if err := features.SupportsFexit(memcgFlushHookName); err != nil {
		return fmt.Errorf("fexit on %s: %w", memcgFlushHookName, err)
	}
	return nil
}

// attachMemcgFlushPrograms attaches the memcg flush programs, if they are loaded.
//
// The pinned links of a previous probe are taken over when both run the same programs on the same
// memcg_flush_seq. Otherwise new links are attached before the pinned ones are released, so that
// the counter is always updated: while both run, the second program of a pair finds the parity
// already changed and leaves the counter as is.
func (k *Probe) attachMemcgFlushPrograms() error {
	// link pin name of each memcg flush program
	linkNames := map[string]string{
		memcgFlushBeginName: memcgFlushBeginLinkName,
		memcgFlushEndName:   memcgFlushEndLinkName,
	}

	progs := make(map[string]*bpflib.Program)
	for name := range linkNames {
		if prog, ok := k.coll.Programs[name]; ok {
			progs[name] = prog
		}
	}
	if len(progs) == 0 {
		return nil
	}
	if len(progs) != len(linkNames) {
		return fmt.Errorf("memcg flush programs %s and %s are not all loaded", memcgFlushBeginName, memcgFlushEndName)
	}
	seq := k.coll.Maps[memcgFlushSeqName]

	pinnedLinks := make(map[string]link.Link)
	pinnedProgs := make(map[string]*bpflib.Program)
	for name, prog := range progs {
		l := loadPinnedLink(linkNames[name])
		if l == nil {
			continue
		}
		pinnedLinks[name] = l
		if pinnedProg := samePinnedProgram(l, prog, seq); pinnedProg != nil {
			pinnedProgs[name] = pinnedProg
		}
	}

	if len(pinnedProgs) == len(progs) {
		log.Infof("taking ownership of pinned memcg flush probes")
		for name, pinnedProg := range pinnedProgs {
			_ = progs[name].Close()
			k.coll.Programs[name] = pinnedProg
			k.addMemcgFlushLink(linkNames[name], pinnedLinks[name])
		}
		return nil
	}

	for _, pinnedProg := range pinnedProgs {
		_ = pinnedProg.Close()
	}
	// give up ownership over the pinned links, after attaching the new ones, so they can be fully
	// released once their pins are removed
	defer func() {
		for _, l := range pinnedLinks {
			_ = l.Close()
		}
	}()

	for name, prog := range progs {
		l, err := link.AttachTracing(link.TracingOptions{Program: prog})
		if err != nil {
			k.closeMemcgFlushLinks()
			return fmt.Errorf("failed to attach %s to %s: %w", name, memcgFlushHookName, err)
		}
		k.addMemcgFlushLink(linkNames[name], l)
	}
	return nil
}

func (k *Probe) addMemcgFlushLink(linkName string, l link.Link) {
	if k.flushLinks == nil {
		k.flushLinks = make(map[string]link.Link)
	}
	k.flushLinks[linkName] = l
}

// loadPinnedLink loads the link pinned for name, if any. A pin that cannot be loaded is removed.
func loadPinnedLink(name string) link.Link {
	l, err := link.LoadPinnedLink(linkPinPath(name), nil)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warnf("failed to load pinned link '%s', removing it: %s", name, err)
			removePin(linkPinPath(name))
		}
		return nil
	}
	return l
}

func (k *Probe) closeMemcgFlushLinks() {
	for _, l := range k.flushLinks {
		if err := l.Close(); err != nil {
			log.Errorf("error detaching memcg flush probe: %s", err)
		}
	}
	k.flushLinks = nil
}

// rewriteConstant sets the value loaded by every LOAD_CONSTANT(name) of the programs in spec.
func rewriteConstant(spec *bpflib.CollectionSpec, name string, value uint64) {
	ldDWImm := asm.LoadImmOp(asm.DWord)
	for _, p := range spec.Programs {
		for i := range p.Instructions {
			ins := &p.Instructions[i]
			if ins.Reference() == name && ins.OpCode == ldDWImm {
				ins.Constant = int64(value)
			}
		}
	}
}

// selectProgram keeps the fentry program in spec if the kernel supports fentry on oom_kill_process
// and the function pointer loads of the fentry program, and the kprobe program otherwise.
// Both run the same logic. Fentry is preferred because its link can be pinned from 5.7 instead of
// 5.15 for the kprobe's perf event link.
func selectProgram(spec *bpflib.CollectionSpec) (*bpflib.ProgramSpec, error) {
	progName, unused := oomKprobeName, oomFentryName
	if err := supportsFentry(oomHookName); err != nil {
		log.Debugf("fentry is not supported on %s, using kprobe: %s", oomHookName, err)
	} else if err := funcPointerLoadSupported(); err != nil {
		log.Debugf("function pointer loads are not supported, using kprobe on %s: %s", oomHookName, err)
	} else {
		progName, unused = oomFentryName, oomKprobeName
	}

	if s, ok := spec.Programs[progName]; ok {
		delete(spec.Programs, unused)
		return s, nil
	}

	return nil, fmt.Errorf("program '%s' not found", progName)
}

// funcPointerLoadSupported loads a program with an unreachable load of the address of one of its
// functions, like the load of the bpf_loop callback of the cgroup memory stats in the fentry
// program when they are not collected. On older kernels check for ldimm64 is performed before
// deadcode elimination so we must ensure that callbacks are supported before attempting loads on
// these kernels.
func funcPointerLoadSupported() error {
	const callback = "callback"
	prog, err := bpflib.NewProgramWithOptions(&bpflib.ProgramSpec{
		Type: bpflib.SocketFilter,
		Instructions: asm.Instructions{
			asm.Mov.Imm(asm.R0, 0),
			asm.JEq.Imm(asm.R0, 0, "exit"),
			asm.Instruction{
				OpCode:   asm.LoadImmOp(asm.DWord),
				Dst:      asm.R2,
				Src:      asm.PseudoFunc,
				Constant: -1,
			}.WithReference(callback),
			asm.Return().WithSymbol("exit"),
			asm.Mov.Imm(asm.R0, 0).WithSymbol(callback),
			asm.Return(),
		},
		License: "GPL",
	}, bpflib.ProgramOptions{LogDisabled: true})
	if err != nil {
		return fmt.Errorf("load test program: %w", err)
	}
	return prog.Close()
}

// Close releases all associated resources.
//
// On a graceful stop the probe is unpinned and detached, so that it does not outlive the module.
// It only keeps running when system-probe dies without calling Close (e.g. when it is OOM killed).
func (k *Probe) Close() {
	ebpf.RemoveNameMappingsCollection(k.coll)
	if k.pinned {
		k.unpin()
	}
	if err := k.link.Close(); err != nil {
		log.Errorf("error detaching OOM Kill probe: %s", err)
	}
	k.closeMemcgFlushLinks()
	k.coll.Close()
}

// GetAndFlush gets the stats
func (k *Probe) GetAndFlush() (results []model.OOMKillStats) {
	var allKeys []statsKey
	var ts statsKey
	var stat oomStats
	it := k.oomMap.Iterate()
	for it.Next(&ts, &stat) {
		if out, err := convertStats(stat, k.emitStats); err != nil {
			log.Error(err)
		} else {
			results = append(results, out)
		}
		allKeys = append(allKeys, ts)
	}

	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate on OOM stats while flushing: %s", err)
	}

	for _, ts := range allKeys {
		if err := k.oomMap.Delete(&ts); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	return results
}

// convertStats converts a map value according to the version of its layout. It returns an error for
// an unknown version, whose value cannot be decoded. The cgroup memory stats are only reported for
// a cgroup OOM kill when emitStats is true.
func convertStats(in oomStats, emitStats bool) (model.OOMKillStats, error) {
	switch v := any(in.Oom_stats_version).(type) {
	case oomStatsV2:
		return convertStatsV2(oomStatsV2Layout(in), emitStats), nil
	default:
		return model.OOMKillStats{}, fmt.Errorf("unsupported oom_stats version %T, dropping OOM kill of pid %d", v, in.Victim_pid)
	}
}

func convertStatsV2(in oomStatsV2Layout, emitStats bool) (out model.OOMKillStats) {
	out.CgroupName = unix.ByteSliceToString(in.Cgroup_name[:])
	out.VictimPid = in.Victim_pid
	out.TriggerPid = in.Trigger_pid
	out.Score = in.Score
	out.ScoreAdj = in.Score_adj
	out.VictimComm = unix.ByteSliceToString(in.Victim_comm[:])
	out.TriggerComm = unix.ByteSliceToString(in.Trigger_comm[:])
	out.Pages = in.Pages
	out.MemCgOOM = in.Memcg_oom
	if !emitStats || in.Memcg_oom != 1 {
		return
	}
	out.CgroupMemory = &model.CgroupMemoryStats{
		Usage:        cgroupMemoryStat(in.Mem.Usage),
		ActiveAnon:   cgroupMemoryStat(in.Mem.Active_anon),
		InactiveAnon: cgroupMemoryStat(in.Mem.Inactive_anon),
		ActiveFile:   cgroupMemoryStat(in.Mem.Active_file),
		InactiveFile: cgroupMemoryStat(in.Mem.Inactive_file),
		Unevictable:  cgroupMemoryStat(in.Mem.Unevictable),
		Kernel:       cgroupMemoryStat(in.Mem.Kernel),
		Unstable:     in.Mem.Flags&cgroupMemStatsUnstable != 0,
	}
	return
}

// cgroupMemoryStat converts a cgroup memory stat from pages, as reported by the eBPF program, to
// bytes. Untracked stat items are reported as 0.
func cgroupMemoryStat(v uint64) uint64 {
	if v == math.MaxUint64 {
		return 0
	}
	return v * pageSize()
}

// pageSize returns the page size of the host, which is the one the kernel counts pages in
var pageSize = sync.OnceValue(func() uint64 {
	return uint64(os.Getpagesize())
})

// pinningFeaturesSupported reports whether the probe should be pinned to bpffs. All of the following must hold:
//   - oom_kill.persist_monitoring is enabled;
//   - bpffs is mounted at bpffsRoot;
//   - the fentry program is in use, its link can be pinned on this kernel, and the info of a
//     pinned link can be read from its fd.
//
// The kprobe program is never pinned: its perf event link can only be pinned from 5.15
func pinningFeaturesSupported(progSpec *bpflib.ProgramSpec) bool {
	if !bpffsAvailable() {
		log.Warnf("bpffs is not mounted at %s, oom-kill probe will not be pinned", bpffsRoot)
		return false
	}
	if progSpec.Type != bpflib.Tracing {
		log.Debugf("oom-kill probe is using a kprobe, which is not pinned")
		return false
	}
	if err := fentryPinningSupported(); err != nil {
		log.Warnf("pinning fentry links is not supported, oom-kill probe will not be pinned: %s", err)
		return false
	}
	return true
}

// fentryPinningSupported attaches a dummy fentry program to oom_kill_process, pins its link in a
// temporary bpffs directory and reads the info of the link through the pin, then removes everything.
func fentryPinningSupported() error {
	prog, err := bpflib.NewProgramWithOptions(&bpflib.ProgramSpec{
		Type:       bpflib.Tracing,
		AttachType: bpflib.AttachTraceFEntry,
		AttachTo:   oomHookName,
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
		License: "GPL",
	}, bpflib.ProgramOptions{LogDisabled: true})
	if err != nil {
		return fmt.Errorf("load test program: %w", err)
	}
	defer prog.Close()

	l, err := link.AttachTracing(link.TracingOptions{Program: prog})
	if err != nil {
		return fmt.Errorf("attach test program: %w", err)
	}
	defer l.Close()

	dir, err := os.MkdirTemp(bpffsRoot, "datadog-agent-oom-kill-pin-test-")
	if err != nil {
		return fmt.Errorf("create test pin directory: %w", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "link")
	if err := l.Pin(path); err != nil {
		return fmt.Errorf("pin test link: %w", err)
	}

	// The next system-probe opens the pinned link and reads its info from the fd to take it over,
	// and Close does the same to check that it owns the pin. Without link info (before 5.8), a
	// pinned probe could neither be taken over nor unpinned.
	pinned, err := link.LoadPinnedLink(path, nil)
	if err != nil {
		return fmt.Errorf("load pinned test link: %w", err)
	}
	defer pinned.Close()
	if _, err := pinned.Info(); err != nil {
		return fmt.Errorf("read info of pinned test link: %w", err)
	}
	return l.Unpin()
}

// bpffsAvailable reports whether bpffsRoot is a mounted bpf filesystem
func bpffsAvailable() bool {
	var st unix.Statfs_t
	if err := unix.Statfs(bpffsRoot, &st); err != nil {
		return false
	}
	return st.Type == unix.BPF_FS_MAGIC
}

// samePinnedProgram returns the program attached to the pinned link if it is identical to prog
// and writes to the pinned map, which is then reused. Returns nil otherwise.
func samePinnedProgram(pinned link.Link, prog *bpflib.Program, maps ...*bpflib.Map) *bpflib.Program {
	if pinned == nil {
		return nil
	}
	linkInfo, err := pinned.Info()
	if err != nil {
		return nil
	}
	pinnedProg, err := bpflib.NewProgramFromID(linkInfo.Program)
	if err != nil {
		return nil
	}

	pinnedInfo, err := pinnedProg.Info()
	if err == nil {
		var progInfo *bpflib.ProgramInfo
		progInfo, err = prog.Info()
		// The kernel computes the tag over the instructions with map references zeroed out
		if err == nil && pinnedInfo.Tag != "" && pinnedInfo.Tag == progInfo.Tag && usesSameMaps(pinnedInfo, maps) {
			return pinnedProg
		}
	}
	_ = pinnedProg.Close()
	return nil
}

// usesSameMaps reports whether the program described by `info` references `maps`
func usesSameMaps(info *bpflib.ProgramInfo, maps []*bpflib.Map) bool {
	ids, ok := info.MapIDs()
	if !ok {
		return false
	}

	for _, m := range maps {
		mapInfo, err := m.Info()
		if err != nil {
			return false
		}
		mapID, ok := mapInfo.ID()
		if !ok {
			return false
		}

		if !slices.Contains(ids, mapID) {
			return false
		}
	}

	return true
}

// removePins removes the link and map pins, and the pin directory if it is empty.
//
// The oom-kill link is removed first and memcg_flush_seq last, so that the oom-kill program never
// outlives the memcg flush programs updating the counter it checks the cgroup memory stats against.
func removePins() {
	removePin(linkPinPath(oomLinkName))
	removePin(mapPinPath(oomMapName))
	removePin(linkPinPath(memcgFlushBeginLinkName))
	removePin(linkPinPath(memcgFlushEndLinkName))
	removePin(mapPinPath(memcgFlushSeqName))
	if err := os.Remove(pinDir); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ENOTEMPTY) {
		log.Debugf("failed to remove pin directory %s: %s", pinDir, err)
	}
}

func removePin(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warnf("failed to remove pin %s: %s", path, err)
	}
}

func linkPinPath(name string) string { return filepath.Join(pinDir, name+"_link") }
func mapPinPath(name string) string  { return filepath.Join(pinDir, name) }
