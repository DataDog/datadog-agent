// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package loader supports setting up the eBPF program.
package loader

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Loader is responsible for loading the eBPF, making it ready to attach.
type Loader struct {
	config

	// Shared ringbuffer for collecting probe output
	ringbufMap    *ebpf.Map
	ringbufReader *ringbuf.Reader

	// Side-channel ringbuffer for drop notifications. Carries fixed-size
	// messages informing userspace that an event couldn't be submitted on
	// the main ringbuffer, so userspace can salvage or discard the
	// associated buffered state rather than leak it.
	dropNotifyMap    *ebpf.Map
	dropNotifyReader *ringbuf.Reader

	ebpfSpec *ebpf.CollectionSpec
}

// WithEbpfConfig sets the eBPF configuration for the compiler.
func WithEbpfConfig(cfg *ddebpf.Config) Option {
	return (*ebpfConfigOption)(cfg)
}

// WithRingBufSize sets the size of the ring buffer for the Actuator.
func WithRingBufSize(size uint32) Option {
	return ringBufSizeOption(size)
}

// WithDebugLevel sets the debug level for the ebpf program.
// It forces use of a binary compiled in a debug mode
func WithDebugLevel(level int) Option {
	return debugLevelOption(level)
}

// WithAdditionalSerializer sets an additional serializer for the ebpf program.
func WithAdditionalSerializer(serializer compiler.CodeSerializer) Option {
	return additionalSerializerOption{serializer}
}

// WithForceMultiAttach forces the loader to use uprobe_multi attachment,
// bypassing the kernel-version gate in canUseMultiAttach. Intended for use
// in tests on kernels that support BPF_LINK_TYPE_UPROBE_MULTI but are
// excluded by the conservative 6.10 floor (e.g. Ubuntu 24.04's 6.8 kernel).
// The caller is responsible for ensuring the workload is unaffected by the
// pre-6.10 multi-uprobe PID-filter bug — see canUseMultiAttach for details.
func WithForceMultiAttach() Option {
	return forceMultiAttachOption{}
}

// NewLoader creates a new Loader.
func NewLoader(opts ...Option) (*Loader, error) {
	l := &Loader{}
	err := l.init(opts...)
	if err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Load loads the program.
func (l *Loader) Load(program compiler.Program) (*Program, error) {
	serialized, err := serializeProgram(program, l.additionalSerializer)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize program: %w", err)
	}

	spec := l.ebpfSpec.Copy()

	maps, err := l.loadData(serialized, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}
	defer func() {
		for k, m := range maps {
			if k == ringbufMapName || k == dropNotifyMapName {
				continue
			}
			_ = m.Close()
		}
	}()

	ringbufMapSpec, ok := spec.Maps[ringbufMapName]
	if !ok {
		return nil, errors.New("ringbuffer map not found in eBPF spec")
	}
	ringbufMapSpec.MaxEntries = uint32(l.config.ringBufSize)

	useMultiAttach := l.config.forceMultiAttach || canUseMultiAttach()
	if useMultiAttach {
		progSpec, ok := spec.Programs["probe_run_with_cookie"]
		if !ok {
			return nil, errors.New("probe_run_with_cookie program not found in eBPF spec")
		}
		progSpec.AttachType = ebpf.AttachTraceUprobeMulti
	}

	opts := ebpf.CollectionOptions{}
	opts.MapReplacements = maps
	opts.MapReplacements[ringbufMapName] = l.ringbufMap
	opts.MapReplacements[dropNotifyMapName] = l.dropNotifyMap
	collection, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("failed to create collection: %w\n%+v", err, ve)
		}
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}
	bpfProgram, ok := collection.Programs["probe_run_with_cookie"]
	if !ok {
		return nil, errors.New("probe_run_with_cookie program not found in collection")
	}

	maps = nil
	return &Program{
		Collection:     collection,
		BpfProgram:     bpfProgram,
		Attachpoints:   serialized.bpfAttachPoints,
		UseMultiAttach: useMultiAttach,
	}, nil
}

// canUseMultiAttach reports whether the running kernel supports
// uprobe_multi attachment with a working PID filter.
//
// Linux 6.6 introduced BPF_LINK_TYPE_UPROBE_MULTI, but its PID filter
// was buggy until 6.10: uprobe_prog_run() compared `current` against the
// per-link task_struct pointer rather than its mm, so probes only fired
// for the single thread looked up at attach time. Go programs run
// goroutines across many OS threads, so most events were silently dropped.
// The fix is upstream commit 46ba0e49b642 ("bpf: fix multi-uprobe PID
// filtering logic"), present in 6.10+ and backported to 6.9.9, but never
// to linux-6.8.y — so Ubuntu 24.04's stock 6.8 kernel is permanently
// affected. Gate on 6.10 to be safe.
var canUseMultiAttach = sync.OnceValue(func() bool {
	if features.HaveBPFLinkUprobeMulti() != nil {
		return false
	}
	v, err := kernel.HostVersion()
	if err != nil {
		return false
	}
	return v >= kernel.VersionCode(6, 10, 0)
})

// stripRelocations removes the relocation metadata from the instructions.
// These are not needed for pt_regs as long as we're not trying to build
// cross-architecture programs (which we're not).
func stripRelocations(spec *ebpf.CollectionSpec) {
	for _, p := range spec.Programs {
		for i := range p.Instructions {
			insn := &p.Instructions[i]
			relo := btf.CORERelocationMetadata(insn)
			if relo == nil {
				continue
			}
			// These are the other metadata fields that we want to keep.
			// See [1] for the fields we decide to keep.
			//
			// [1]: https://github.com/cilium/ebpf/blob/49ae13c6/btf/ext_info.go#L119-L125
			funcMetadata := btf.FuncMetadata(insn)
			source := insn.Source()
			*insn = insn.WithMetadata(asm.Metadata{})
			*insn = btf.WithFuncMetadata(*insn, funcMetadata)
			*insn = insn.WithSource(source)
		}
	}
}

// OutputReader returns the ringbuffer reader for the loader.
func (l *Loader) OutputReader() *ringbuf.Reader {
	if l.ringbufReader == nil {
		panic("ringbuffer reader not initialized")
	}
	return l.ringbufReader
}

// DropNotifyReader returns the side-channel ringbuffer reader carrying
// drop notifications.
func (l *Loader) DropNotifyReader() *ringbuf.Reader {
	if l.dropNotifyReader == nil {
		panic("drop-notify ringbuffer reader not initialized")
	}
	return l.dropNotifyReader
}

// Close releases loader resources.
func (l *Loader) Close() (err error) {
	if l.ringbufReader != nil {
		err = errors.Join(err, l.ringbufReader.Close())
	}
	if l.ringbufMap != nil {
		err = errors.Join(err, l.ringbufMap.Close())
	}
	if l.dropNotifyReader != nil {
		err = errors.Join(err, l.dropNotifyReader.Close())
	}
	if l.dropNotifyMap != nil {
		err = errors.Join(err, l.dropNotifyMap.Close())
	}
	return err
}

// Program represents a loaded eBPF program.
type Program struct {
	Collection   *ebpf.Collection
	BpfProgram   *ebpf.Program
	Attachpoints []BPFAttachPoint
	// UseMultiAttach indicates that the program was loaded with
	// AttachTraceUprobeMulti and should be attached using
	// link.Executable.UprobeMulti rather than per-address Uprobe calls.
	UseMultiAttach bool
}

// Close releases the program resources.
func (p *Program) Close() {
	if p.Collection != nil {
		p.Collection.Close() // should already contain the program
	}
}

// RuntimeStats are cumulative stats aggregated throughout probe lifetime.
type RuntimeStats struct {
	// Aggregated cpu time spent in probe execution (excluding interrupt overhead).
	CPU time.Duration
	// Number of probe hits.
	HitCnt uint64
	// Number of probe hits that skipped data capture due to throttling.
	ThrottledCnt uint64

	// runtime.recovery probe counters. Zero on programs where no
	// FunctionWhere user probe is configured (the recovery probe is
	// not synthesised; see irgen.maybeAddRuntimeRecoveryProbe).
	// Recovery counters are written only on the probe-0 entry of the
	// stats_buf ARRAY (they are process-wide, not per-probe).

	// RecoveryFires is the number of times the runtime.recovery uprobe
	// fired. Each firing corresponds to one panic+recover that reached
	// runtime.recovery.
	RecoveryFires uint64
	// RecoveryEvictedFrames is the cumulative number of in_progress_calls
	// slots evicted by recovery firings. A single recovery can evict 0
	// or more frames depending on how many probed frames sit in the
	// unwound region.
	RecoveryEvictedFrames uint64
	// RecoverySubmitFailures is the number of synthetic-event submits
	// that failed (out_ringbuf full) and were converted to a
	// PANIC_UNWOUND_LOST drop notification on the side channel.
	RecoverySubmitFailures uint64
	// RecoveryNoOpenCalls counts recoveries on goroutines that had no
	// probed-frame pairing state in flight. These are common (every
	// non-probed goroutine that panics+recovers) and very cheap thanks
	// to an early short-circuit before the panic-chain reads.
	RecoveryNoOpenCalls uint64
	// RecoveryFilteredGoexit counts recoveries triggered by a
	// runtime.Goexit unwind rather than a real panic-recover; these are
	// out of scope for this revision and are skipped.
	RecoveryFilteredGoexit uint64
	// RecoveryInvalidState counts recoveries we couldn't process due
	// to defensive bail-out conditions (panic_ptr==0, recovered!=1,
	// stack_hi mismatch, etc.). Should normally stay at 0; a non-zero
	// value indicates either a runtime.recovery firing pattern this
	// code doesn't understand or a DWARF/ABI mismatch.
	RecoveryInvalidState uint64
}

// RuntimeStats returns the per-probe runtime stats for the program,
// indexed by the IR probe_id (the same value used as stats_buf key in
// eBPF). The returned slice has length equal to the program's probe
// count.
func (p *Program) RuntimeStats() []RuntimeStats {
	statsMap, ok := p.Collection.Maps["stats_buf"]
	if !ok {
		return nil
	}
	n := int(statsMap.MaxEntries())
	out := make([]RuntimeStats, n)
	if n == 0 {
		return out
	}
	// stats and RuntimeStats have the same layout — see
	// TestRuntimeStatsHasSameLayoutAsStats. Read directly into the
	// output slice as []stats.
	view := unsafe.Slice(
		(*stats)(unsafe.Pointer(unsafe.SliceData(out))),
		n,
	)
	entries := statsMap.Iterate()
	var key uint32
	var value stats
	for entries.Next(&key, &value) {
		if int(key) >= n {
			continue
		}
		view[key] = value
	}
	return out
}

const defaultRingbufSize = 1 << 20 // 1 MiB
const ringbufMapName = "out_ringbuf"

// Side-channel ringbuf. Small (16 KiB ~= 500 notifications); notifications
// are fixed-size so it very rarely fills under pressure. Kept in sync with
// DROP_NOTIFY_RINGBUF_CAPACITY in ../ebpf/scratch.h.
const defaultDropNotifyRingbufSize = 1 << 14
const dropNotifyMapName = "drop_notify_ringbuf"

// dropNotifyLostAtMapName is a single-slot BPF_MAP_TYPE_ARRAY holding the
// most recent ktime_ns at which the BPF side failed to publish a drop
// notification (drop_notify_ringbuf full). Userspace polls it to drive
// eventbuf eviction. See pkg/dyninst/ebpf/scratch.h.
const dropNotifyLostAtMapName = "drop_notify_lost_at"

// DropNotifyLostAt returns the kernel-monotonic ktime_ns of the most
// recent in-BPF attempt to publish a drop notification that failed
// because the side-channel ringbuf was full. Returns 0 if no failure has
// ever been recorded for this program (or if the map is unavailable).
func (p *Program) DropNotifyLostAt() uint64 {
	m, ok := p.Collection.Maps[dropNotifyLostAtMapName]
	if !ok {
		return 0
	}
	var key uint32
	var val uint64
	if err := m.Lookup(&key, &val); err != nil {
		return 0
	}
	return val
}

type config struct {
	ebpfConfig *ddebpf.Config

	ringBufSize uint32

	dyninstDebugLevel   uint8
	dyninstDebugEnabled bool

	forceMultiAttach bool

	additionalSerializer compiler.CodeSerializer
}

// Option configures the Loader.
type Option interface {
	apply(c *config)
}

type ebpfConfigOption ddebpf.Config

func (o *ebpfConfigOption) apply(c *config) {
	c.ebpfConfig = (*ddebpf.Config)(o)
}

type ringBufSizeOption uint32

func (o ringBufSizeOption) apply(c *config) {
	c.ringBufSize = uint32(o)
}

type debugLevelOption uint8

func (o debugLevelOption) apply(c *config) {
	c.dyninstDebugLevel = uint8(o)
	c.dyninstDebugEnabled = true
}

type additionalSerializerOption struct {
	compiler.CodeSerializer
}

func (o additionalSerializerOption) apply(c *config) {
	c.additionalSerializer = o
}

type forceMultiAttachOption struct{}

func (forceMultiAttachOption) apply(c *config) {
	c.forceMultiAttach = true
}

func (l *Loader) init(opts ...Option) error {
	l.config.ringBufSize = defaultRingbufSize
	for _, opt := range opts {
		opt.apply(&l.config)
	}
	if l.config.ebpfConfig == nil {
		l.config.ebpfConfig = ddebpf.NewConfig()
	}
	var err error
	l.ringbufMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Name:       ringbufMapName,
		Type:       ebpf.RingBuf,
		MaxEntries: uint32(l.config.ringBufSize),
	})
	if err != nil {
		return fmt.Errorf("failed to create ringbuffer map: %w", err)
	}
	l.ringbufReader, err = ringbuf.NewReader(l.ringbufMap)
	if err != nil {
		return fmt.Errorf("failed to create ringbuffer reader: %w", err)
	}
	l.dropNotifyMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Name:       dropNotifyMapName,
		Type:       ebpf.RingBuf,
		MaxEntries: defaultDropNotifyRingbufSize,
	})
	if err != nil {
		return fmt.Errorf("failed to create drop-notify ringbuffer map: %w", err)
	}
	l.dropNotifyReader, err = ringbuf.NewReader(l.dropNotifyMap)
	if err != nil {
		return fmt.Errorf("failed to create drop-notify ringbuffer reader: %w", err)
	}
	obj, err := getBpfObject(&l.config)
	if err != nil {
		return fmt.Errorf("failed to get eBPF object: %w", err)
	}
	defer obj.Close()
	l.ebpfSpec, err = ebpf.LoadCollectionSpecFromReader(obj)
	if err != nil {
		return fmt.Errorf("failed to load eBPF object: %w", err)
	}
	stripRelocations(l.ebpfSpec)
	ringbufMapSpec, ok := l.ebpfSpec.Maps[ringbufMapName]
	if !ok {
		return errors.New("ringbuffer map not found in eBPF spec")
	}
	ringbufMapSpec.MaxEntries = uint32(l.config.ringBufSize)
	if _, ok := l.ebpfSpec.Maps[dropNotifyMapName]; !ok {
		return errors.New("drop-notify ringbuffer map not found in eBPF spec")
	}
	return nil
}

func getBpfObject(cfg *config) (bytecode.AssetReader, error) {
	baseDir := filepath.Join(cfg.ebpfConfig.BPFDir, "co-re")
	if cfg.dyninstDebugEnabled {
		return bytecode.GetReader(baseDir, "dyninst_event-debug.o")
	}
	return bytecode.GetReader(baseDir, "dyninst_event.o")
}

func (l *Loader) loadData(
	serialized *serializedProgram,
	spec *ebpf.CollectionSpec,
) (map[string]*ebpf.Map, error) {
	const codeMapName = "stack_machine_code"
	const typeIDsMapName = "type_ids"
	const typeInfoMapName = "type_info"
	const throttlerMapName = "throttler_params"
	const throttlerStateMapName = "throttler_buf"
	const probeParamsMapName = "probe_params"
	const statsBufMapName = "stats_buf"
	const goRuntimeTypeIDsMapName = "go_runtime_type_ids"
	const goRuntimeTypesMapName = "go_runtime_types"

	mapSpec, codeMap, err := makeArrayMap(codeMapName, serialized.code, forceSingleEntryMap)
	spec.Maps[codeMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create code map: %w", err)
	}
	defer func() {
		if codeMap != nil {
			codeMap.Close()
		}
	}()
	err = setVariable(spec, "stack_machine_code_len", uint32(len(serialized.code)))
	if err != nil {
		return nil, fmt.Errorf("failed to set stack_machine_code_len: %w", err)
	}
	err = setVariable(spec, "stack_machine_code_max_op", serialized.maxOpLen)
	if err != nil {
		return nil, fmt.Errorf("failed to set stack_machine_code_max_op: %w", err)
	}
	err = setVariable(spec, "chase_pointers_entrypoint", serialized.chasePointersEntrypoint)
	if err != nil {
		return nil, fmt.Errorf("failed to set chase_pointers_entrypoint: %w", err)
	}
	err = setVariable(spec, "prog_id", uint32(serialized.programID))
	if err != nil {
		return nil, fmt.Errorf("failed to set prog_id: %w", err)
	}

	mapSpec, typeIDsMap, err := makeArrayMap(
		typeIDsMapName, serialized.typeIDs, allowMultipleMapEntries,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create type_ids map: %w", err)
	}
	spec.Maps[typeIDsMapName] = mapSpec
	defer func() {
		if typeIDsMap != nil {
			typeIDsMap.Close()
		}
	}()
	err = setVariable(spec, "num_types", uint32(len(serialized.typeIDs)))
	if err != nil {
		return nil, fmt.Errorf("failed to set num_types: %w", err)
	}

	mapSpec, typeInfoMap, err := makeArrayMap(
		typeInfoMapName, serialized.typeInfos, allowMultipleMapEntries,
	)
	spec.Maps[typeInfoMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create type_info map: %w", err)
	}
	defer func() {
		if typeInfoMap != nil {
			typeInfoMap.Close()
		}
	}()
	grts := &serialized.goRuntimeTypeIDs
	numGoRuntimeTypes := uint32(grts.Len())
	if numGoRuntimeTypes == 0 {
		// We're not allowed to have empty maps, so we set a single element with
		// a zero value, but the associated variable for the length will still
		// be set to zero.
		grts.goRuntimeTypes = []uint64{0}
		grts.typeIDs = []uint64{0}
		grts.directTypeIDs = []uint64{0}
	}
	goRuntimeTypeIDsMapSpec, goRuntimeTypeIDsMap, err := makeArrayMap(
		goRuntimeTypeIDsMapName, grts.typeIDs, allowMultipleMapEntries,
	)
	spec.Maps[goRuntimeTypeIDsMapName] = goRuntimeTypeIDsMapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create go_runtime_type_ids map: %w", err)
	}
	defer func() {
		if goRuntimeTypeIDsMap != nil {
			goRuntimeTypeIDsMap.Close()
		}
	}()
	goRuntimeTypesMapSpec, goRuntimeTypesMap, err := makeArrayMap(
		goRuntimeTypesMapName, grts.goRuntimeTypes, allowMultipleMapEntries,
	)
	spec.Maps[goRuntimeTypesMapName] = goRuntimeTypesMapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create go_runtime_types map: %w", err)
	}
	defer func() {
		if goRuntimeTypesMap != nil {
			goRuntimeTypesMap.Close()
		}
	}()
	const goRuntimeTypeDirectIDsMapName = "go_runtime_type_direct_ids"
	goRuntimeTypeDirectIDsMapSpec, goRuntimeTypeDirectIDsMap, err := makeArrayMap(
		goRuntimeTypeDirectIDsMapName, grts.directTypeIDs, allowMultipleMapEntries,
	)
	spec.Maps[goRuntimeTypeDirectIDsMapName] = goRuntimeTypeDirectIDsMapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create go_runtime_type_direct_ids map: %w", err)
	}
	defer func() {
		if goRuntimeTypeDirectIDsMap != nil {
			goRuntimeTypeDirectIDsMap.Close()
		}
	}()
	if err := setVariable(
		spec, "num_go_runtime_types", numGoRuntimeTypes,
	); err != nil {
		return nil, fmt.Errorf("failed to set num_go_runtime_types: %w", err)
	}
	if err := setVariable(
		spec, "trace_context_type_id", uint32(serialized.traceContextTypeID),
	); err != nil {
		return nil, fmt.Errorf("failed to set trace_context_type_id: %w", err)
	}
	// Allow a program to avoid setting common constants if it doesn't have
	// any. This is something of a hack to allow for the rcscrape program to
	// avoid needing constants, and corresponds to similar flexibility in the
	// eBPF program.
	//
	// TODO: Remove this by either fully eliminating the rcscrape eBPF program
	// or fully decoupling it from this program infrastructure.
	if serialized.commonTypes != (ir.CommonTypes{}) {
		if err := setCommonConstants(spec, serialized); err != nil {
			return nil, fmt.Errorf("failed to set common constants: %w", err)
		}
	}

	if err := setMapHashConstants(spec, serialized); err != nil {
		return nil, fmt.Errorf("failed to set map hash constants: %w", err)
	}
	if serialized.isARM64 {
		if err := setVariable(spec, "is_arm64", uint32(1)); err != nil {
			return nil, fmt.Errorf("failed to set is_arm64: %w", err)
		}
	}

	mapSpec, throttlerMap, err := makeArrayMap(
		throttlerMapName, serialized.throttlerParams, allowMultipleMapEntries,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create throttler_params map: %w", err)
	}
	spec.Maps[throttlerMapName] = mapSpec
	defer func() {
		if throttlerMap != nil {
			throttlerMap.Close()
		}
	}()
	if err := setVariable(
		spec, "num_throttlers", uint32(len(serialized.throttlerParams)),
	); err != nil {
		return nil, fmt.Errorf("failed to set num_throttlers: %w", err)
	}

	mapSpec, ok := spec.Maps[throttlerStateMapName]
	if !ok {
		return nil, errors.New("throttler_buf map not found in eBPF spec")
	}
	mapSpec.MaxEntries = uint32(len(serialized.throttlerParams))

	mapSpec, probeParamsMap, err := makeArrayMap(
		probeParamsMapName, serialized.probeParams, allowMultipleMapEntries,
	)
	spec.Maps[probeParamsMapName] = mapSpec
	if err != nil {
		return nil, fmt.Errorf("failed to create probe_params map: %w", err)
	}
	defer func() {
		if probeParamsMap != nil {
			probeParamsMap.Close()
		}
	}()
	err = setVariable(spec, "num_probe_params", uint32(len(serialized.probeParams)))
	if err != nil {
		return nil, fmt.Errorf("failed to set num_probe_params: %w", err)
	}

	// Size stats_buf to one entry per IR probe. BPF_MAP_TYPE_ARRAY
	// requires max_entries >= 1, so clamp degenerate zero-probe
	// programs (the verifier still requires the map to exist).
	statsBufSize := serialized.numProbes
	if statsBufSize == 0 {
		statsBufSize = 1
	}
	statsBufMapSpec, ok := spec.Maps[statsBufMapName]
	if !ok {
		return nil, errors.New("stats_buf map not found in eBPF spec")
	}
	statsBufMapSpec.MaxEntries = statsBufSize

	if l.config.dyninstDebugEnabled {
		err = setVariable(spec, "debug_level", uint32(l.config.dyninstDebugLevel))
		if err != nil {
			return nil, fmt.Errorf("failed to set debug_level: %w", err)
		}
	}

	m := map[string]*ebpf.Map{
		codeMapName:                   codeMap,
		typeIDsMapName:                typeIDsMap,
		typeInfoMapName:               typeInfoMap,
		throttlerMapName:              throttlerMap,
		probeParamsMapName:            probeParamsMap,
		goRuntimeTypeIDsMapName:       goRuntimeTypeIDsMap,
		goRuntimeTypesMapName:         goRuntimeTypesMap,
		goRuntimeTypeDirectIDsMapName: goRuntimeTypeDirectIDsMap,
	}
	codeMap = nil
	typeIDsMap = nil
	typeInfoMap = nil
	throttlerMap = nil
	probeParamsMap = nil
	goRuntimeTypeIDsMap = nil
	goRuntimeTypesMap = nil
	goRuntimeTypeDirectIDsMap = nil
	return m, nil
}

func setCommonConstants(spec *ebpf.CollectionSpec, serialized *serializedProgram) error {
	if err := setVariable(
		spec, "VARIABLE_runtime_dot_firstmoduledata",
		serialized.goModuledataInfo.FirstModuledataAddr,
	); err != nil {
		return err
	}
	if err := setVariable(
		spec, "OFFSET_runtime_dot_moduledata__types",
		serialized.goModuledataInfo.TypesOffset,
	); err != nil {
		return err
	}
	g := serialized.commonTypes.G
	m := serialized.commonTypes.M
	stack, ok := g.FieldByName("stack")
	if !ok {
		return errors.New("stack field not found in runtime.g")
	}
	stackStruct, ok := stack.Type.(*ir.StructureType)
	if !ok {
		return fmt.Errorf("stack field of runtime.g is not a structure type, got %T", stack.Type)
	}
	for _, f := range []struct {
		s            *ir.StructureType
		fieldName    string
		variableName string
	}{
		{m, "curg", "OFFSET_runtime_dot_m__curg"},
		{g, "goid", "OFFSET_runtime_dot_g__goid"},
		{g, "m", "OFFSET_runtime_dot_g__m"},
		{g, "stack", "OFFSET_runtime_dot_g__stack"},
		{stackStruct, "hi", "OFFSET_runtime_dot_stack__hi"},
	} {
		offset, err := f.s.FieldOffsetByName(f.fieldName)
		if err != nil {
			var fields []string
			for field := range f.s.Fields() {
				fields = append(fields, field.Name)
			}
			err = fmt.Errorf(
				"failed to get field offset for %s in %s: %w (fields: %s)",
				f.fieldName, f.s.Name, err, strings.Join(fields, ", "),
			)
			panic(err)
		}

		if err := setVariable(spec, f.variableName, offset); err != nil {
			return fmt.Errorf(
				"failed to set %s for %s in %s: %w",
				f.variableName, f.fieldName, f.s.Name, err,
			)
		}
	}

	// runtime._panic offsets. Optional: if the binary's DWARF lacks the
	// type or a field, leave the corresponding OFFSET_ at 0 — the
	// recovery probe attach path treats a missing g._panic offset as a
	// signal to skip the probe and the rest of dyninst continues working.
	panicStruct := serialized.commonTypes.Panic
	if panicStruct == nil {
		log.Warnf(
			"dyninst: runtime._panic not present in target DWARF; " +
				"recovery probe will no-op and panic-recover leaks will " +
				"not be cleaned up for this binary",
		)
		return nil
	}
	gPanicOff, err := g.FieldOffsetByName("_panic")
	if err != nil {
		log.Warnf(
			"dyninst: runtime.g has no _panic field (%v); recovery "+
				"probe will no-op", err,
		)
		return nil
	}
	if err := setVariable(spec, "OFFSET_runtime_dot_g___panic", gPanicOff); err != nil {
		return fmt.Errorf("failed to set OFFSET_runtime_dot_g___panic: %w", err)
	}
	var missing []string
	for _, f := range []struct {
		fieldName    string
		variableName string
	}{
		{"arg", "OFFSET_runtime_dot__panic__arg"},
		{"startSP", "OFFSET_runtime_dot__panic__startSP"},
		{"sp", "OFFSET_runtime_dot__panic__sp"},
		{"recovered", "OFFSET_runtime_dot__panic__recovered"},
		{"goexit", "OFFSET_runtime_dot__panic__goexit"},
	} {
		off, err := panicStruct.FieldOffsetByName(f.fieldName)
		if err != nil {
			missing = append(missing, f.fieldName)
			continue
		}
		if err := setVariable(spec, f.variableName, off); err != nil {
			return fmt.Errorf(
				"failed to set %s for %s in runtime._panic: %w",
				f.variableName, f.fieldName, err,
			)
		}
	}
	if len(missing) > 0 {
		log.Warnf(
			"dyninst: runtime._panic missing fields %v; recovery probe "+
				"may behave incorrectly (panic-unwound frames could leak)",
			missing,
		)
	}
	return nil
}

func setMapHashConstants(spec *ebpf.CollectionSpec, serialized *serializedProgram) error {
	info := serialized.goMapHashInfo
	// These are optional — only set if the addresses were found in DWARF.
	// If not found (zero), the BPF code will not be able to compute hashes
	// and map index expressions will fail gracefully at runtime.
	if info.UseAeshashAddr != 0 {
		if err := setVariable(spec, "VARIABLE_runtime_dot_useAeshash", info.UseAeshashAddr); err != nil {
			return err
		}
	}
	if info.AeskeyschedAddr != 0 {
		if err := setVariable(spec, "VARIABLE_runtime_dot_aeskeysched", info.AeskeyschedAddr); err != nil {
			return err
		}
	}
	return nil
}

type arrayMapConfig bool

const (
	forceSingleEntryMap     arrayMapConfig = true
	allowMultipleMapEntries arrayMapConfig = false
)

func makeArrayMap[T any](
	name string, data []T, cfg arrayMapConfig,
) (*ebpf.MapSpec, *ebpf.Map, error) {
	var val T
	elemSize := uint32(unsafe.Sizeof(val))
	mapSpec := &ebpf.MapSpec{
		Name:       name,
		Type:       ebpf.Array,
		KeySize:    4,
		ValueSize:  uint32(elemSize),
		MaxEntries: uint32(len(data)),
		Flags:      features.BPF_F_MMAPABLE,
	}
	// singleEntry makes the map have a single element holding all the data.
	if cfg == forceSingleEntryMap {
		mapSpec.ValueSize = mapSpec.MaxEntries * mapSpec.ValueSize
		mapSpec.MaxEntries = 1
	}
	if mapSpec.ValueSize%8 != 0 && cfg == allowMultipleMapEntries {
		return nil, nil, fmt.Errorf(
			"map %s has value size %d which is not a multiple of 8",
			name, mapSpec.ValueSize,
		)
	}
	m, err := ebpf.NewMap(mapSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create %s map: %w", name, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = m.Close()
		}
	}()
	mem, err := m.Memory()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get %s map memory: %w", name, err)
	}
	bytes := unsafe.Slice(
		(*byte)(unsafe.Pointer(&data[0])),
		int(elemSize)*len(data),
	)
	_, err = mem.WriteAt(bytes, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write %s map: %w", name, err)
	}
	cleanup = false
	return mapSpec, m, nil
}

func setVariable[I uint32 | uint64](
	spec *ebpf.CollectionSpec, name string, value I,
) error {
	vari, ok := spec.Variables[name]
	if !ok {
		return fmt.Errorf("variable %s not found in spec", name)
	}
	err := vari.Set(value)
	if err != nil {
		return fmt.Errorf("failed to set %s: %w", name, err)
	}
	return nil
}
