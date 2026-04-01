// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package tracer contains functionality for populating tracers.
package tracer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/linux"
	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/log"
	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/elastic/go-perf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfunsafe"

	pm "github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/processmanager"
	tracertypes "github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/tracer/types"
	"go.opentelemetry.io/ebpf-profiler/kallsyms"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/xsync"
	"go.opentelemetry.io/ebpf-profiler/metrics"
	"go.opentelemetry.io/ebpf-profiler/nativeunwind/elfunwindinfo"
	"go.opentelemetry.io/ebpf-profiler/periodiccaller"
	pmebpf "go.opentelemetry.io/ebpf-profiler/processmanager/ebpf"
	"go.opentelemetry.io/ebpf-profiler/reporter"
	"go.opentelemetry.io/ebpf-profiler/rlimit"
	"go.opentelemetry.io/ebpf-profiler/support"
	"go.opentelemetry.io/ebpf-profiler/times"
	upstreamtypes "go.opentelemetry.io/ebpf-profiler/tracer/types"
)

// Compile time check to make sure times.Times satisfies the interfaces.
var _ Intervals = (*times.Times)(nil)

const (
	// ProbabilisticThresholdMax defines the upper bound of the probabilistic profiling
	// threshold.
	ProbabilisticThresholdMax = 100
)

// Constants that define the status of probabilistic profiling.
const (
	probProfilingEnable  = 1
	probProfilingDisable = -1
)

// Names of tracepoint hooks for sched_process_free. There are two hooks
// as the tracepoint format has changed for kernel versions 6.16+.
const (
	schedProcessFreeV1 = "tracepoint__sched_process_free_pre616"
	schedProcessFreeV2 = "tracepoint__sched_process_free"
)

// Intervals is a subset of config.IntervalsAndTimers.
type Intervals interface {
	MonitorInterval() time.Duration
	TracePollInterval() time.Duration
	PIDCleanupInterval() time.Duration
	ExecutableUnloadDelay() time.Duration
}

// Tracer provides an interface for loading and initializing the eBPF components as
// well as for monitoring the output maps for new traces and count updates.
type Tracer struct {
	// ebpfMaps holds the currently loaded eBPF maps.
	ebpfMaps map[string]*cebpf.Map
	// ebpfProgs holds the currently loaded eBPF programs.
	ebpfProgs map[string]*cebpf.Program

	// kernelSymbolizer does kernel fallback symbolization
	kernelSymbolizer *kallsyms.Symbolizer

	// perfEntrypoints holds a list of frequency based perf events that are opened on the system.
	perfEntrypoints xsync.RWMutex[[]*perf.Event]

	// hooks holds references to loaded eBPF hooks.
	hooks map[hookPoint]link.Link

	// processManager keeps track of loading, unloading and organization of information
	// that is required to unwind processes in the kernel. This includes maintaining the
	// associated eBPF maps.
	processManager *pm.ProcessManager

	// tracePool is cache of libpf.EbpfTrace to avoid GC pressure
	tracePool sync.Pool

	// monitorPIDEventsMap iterates over the eBPF map pid_events, collects PIDs and
	// writes them to the keys slice. The implementation is selected on creation.
	monitorPIDEventsMapMethod func(keys *[]libpf.PIDTID) error

	// triggerPIDProcessing is used as manual trigger channel to request immediate
	// processing of pending PIDs. This is requested on notifications from eBPF code
	// when process events take place (new, exit, unknown PC).
	triggerPIDProcessing chan bool

	// pidEvents notifies the tracer of new PID events. Each PID event is a 64bit integer
	// value, see bpf_get_current_pid_tgid for information on how the value is encoded.
	// It needs to be buffered to avoid locking the writers and stacking up resources when we
	// read new PIDs at startup or notified via eBPF.
	pidEvents chan libpf.PIDTID

	// intervals provides access to globally configured timers and counters.
	intervals Intervals

	hasBatchOperations bool

	// samplesPerSecond holds the configured number of samples per second.
	samplesPerSecond int

	// probabilisticInterval is the time interval for which probabilistic profiling will be enabled.
	probabilisticInterval time.Duration

	// probabilisticThreshold holds the threshold for probabilistic profiling.
	probabilisticThreshold uint

	// done is closed when the tracer encounters an unrecoverable error.
	// Use Done() to obtain a read-only channel for use in select statements.
	done     chan libpf.Void
	doneOnce sync.Once
}

// Done returns a channel that is closed when the tracer encounters an
// unrecoverable error. It can be used in select statements to detect
// when the tracer should be stopped.
func (t *Tracer) Done() <-chan libpf.Void {
	return t.done
}

// signalDone closes the done channel to indicate an unrecoverable error.
// It is safe to call multiple times.
func (t *Tracer) signalDone() {
	t.doneOnce.Do(func() { close(t.done) })
}

type Config struct {
	// ExecutableReporter allows to configure a ExecutableReporter to hook seen executables.
	// NOTE: This is used by external implementations embedding opentelemtry-ebpf-profiler.
	ExecutableReporter reporter.ExecutableReporter
	// TraceReporter is the interface to report traces with.
	TraceReporter reporter.TraceReporter
	// Intervals provides access to globally configured timers and counters.
	Intervals Intervals
	// IncludeTracers holds information about which tracers are enabled.
	IncludeTracers tracertypes.IncludedTracers
	// SamplesPerSecond holds the number of samples per second.
	SamplesPerSecond int
	// MapScaleFactor is the scaling factor for eBPF map sizes.
	MapScaleFactor int
	// FilterErrorFrames indicates whether error frames should be filtered.
	FilterErrorFrames bool
	// FilterIdleFrames indicates whether idle frames should be filtered.
	FilterIdleFrames bool
	// KernelVersionCheck indicates whether the kernel version should be checked.
	KernelVersionCheck bool
	// VerboseMode indicates whether to enable verbose output of eBPF tracers.
	VerboseMode bool
	// BPFVerifierLogLevel is the log level of the eBPF verifier output.
	BPFVerifierLogLevel uint32
	// ProbabilisticInterval is the time interval for which probabilistic profiling will be enabled.
	ProbabilisticInterval time.Duration
	// ProbabilisticThreshold is the threshold for probabilistic profiling.
	ProbabilisticThreshold uint
	// OffCPUThreshold is the user defined threshold for off-cpu profiling.
	OffCPUThreshold uint32
	// IncludeEnvVars holds a list of environment variables that should be captured and reported
	// from processes
	IncludeEnvVars libpf.Set[string]
	// Probes holds a list of probe_type:target[:symbol] elements to which
	// a probe will be attached.
	ProbeLinks []string
	// LoadProbe indicates whether the generic eBPF program should be loaded
	// without being attached to something.
	LoadProbe bool
}

// hookPoint specifies the group and name of the hooked point in the kernel.
type hookPoint struct {
	group, name string
}

// progLoaderHelper supports the loading process of eBPF programs.
type progLoaderHelper struct {
	// enable tells whether a prog shall be loaded.
	enable bool
	// name of the eBPF program
	name string
	// progID defines the ID for the eBPF program that is used as key in the tailcallMap.
	progID uint32
	// noTailCallTarget indicates if this eBPF program should be added to the tailcallMap.
	noTailCallTarget bool
}

// Convert a C-string to Go string.
func goString(cstr []byte) libpf.String {
	index := bytes.IndexByte(cstr, byte(0))
	if index < 0 {
		index = len(cstr)
	}
	return libpf.Intern(pfunsafe.ToString(cstr[:index]))
}

// schedProcessFreeHookName returns the name of the tracepoint hook to use.
// This function requires that only one of (schedProcessFreeV1, schedProcessFreeV2)
// be present in progNames.
func schedProcessFreeHookName(progNames libpf.Set[string]) string {
	if _, ok := progNames[schedProcessFreeV1]; ok {
		return schedProcessFreeV1
	}
	return schedProcessFreeV2
}

func newTracePool() sync.Pool {
	return sync.Pool{
		New: func() any {
			return &libpf.EbpfTrace{}
		},
	}
}

// NewTracer loads eBPF code and map definitions from the ELF module at the configured path.
func NewTracer(ctx context.Context, cfg *Config) (*Tracer, error) {
	kernelSymbolizer, err := kallsyms.NewSymbolizer()
	if err != nil {
		return nil, fmt.Errorf("failed to read kernel symbols: %v", err)
	}

	kmod, err := kernelSymbolizer.GetModuleByName(kallsyms.Kernel)
	if err != nil {
		return nil, fmt.Errorf("failed to read kernel symbols: %v", err)
	}

	// Based on includeTracers we decide later which are loaded into the kernel.
	ebpfMaps, ebpfProgs, stackdeltaInnerMapSpec, err := initializeMapsAndPrograms(kmod, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load eBPF code: %v", err)
	}

	ebpfHandler, err := pmebpf.LoadMaps(
		ctx,
		upstreamtypes.IncludedTracers(cfg.IncludeTracers),
		ebpfMaps,
		stackdeltaInnerMapSpec,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load eBPF maps: %v", err)
	}

	hasBatchOperations := ebpfHandler.SupportsGenericBatchOperations()
	hasBatchLookupAndDelete := ebpfHandler.SupportsGenericBatchLookupAndDelete()

	processManager, err := pm.New(ctx, cfg.IncludeTracers, cfg.Intervals.MonitorInterval(),
		cfg.Intervals.ExecutableUnloadDelay(), ebpfHandler, cfg.TraceReporter, cfg.ExecutableReporter,
		elfunwindinfo.NewStackDeltaProvider(),
		cfg.FilterErrorFrames, cfg.IncludeEnvVars)
	if err != nil {
		return nil, fmt.Errorf("failed to create processManager: %v", err)
	}

	perfEventList := []*perf.Event{}

	tracer := &Tracer{
		kernelSymbolizer:       kernelSymbolizer,
		processManager:         processManager,
		triggerPIDProcessing:   make(chan bool, 1),
		tracePool:              newTracePool(),
		pidEvents:              make(chan libpf.PIDTID, pidEventBufferSize),
		ebpfMaps:               ebpfMaps,
		ebpfProgs:              ebpfProgs,
		hooks:                  make(map[hookPoint]link.Link),
		intervals:              cfg.Intervals,
		hasBatchOperations:     hasBatchOperations,
		perfEntrypoints:        xsync.NewRWMutex(perfEventList),
		samplesPerSecond:       cfg.SamplesPerSecond,
		probabilisticInterval:  cfg.ProbabilisticInterval,
		probabilisticThreshold: cfg.ProbabilisticThreshold,
		done:                   make(chan libpf.Void),
	}

	// Use an optimized version if available
	if hasBatchLookupAndDelete {
		tracer.monitorPIDEventsMapMethod = (*tracer).monitorPIDEventsMapBatch
	} else {
		tracer.monitorPIDEventsMapMethod = (*tracer).monitorPIDEventsMapSingle
	}

	return tracer, nil
}

// Close provides functionality for Tracer to perform cleanup tasks.
// NOTE: Close may be called multiple times in succession.
func (t *Tracer) Close() {
	events := t.perfEntrypoints.WLock()
	terminatePerfEvents(*events)
	*events = nil
	t.perfEntrypoints.WUnlock(&events)

	// Avoid resource leakage by closing all kernel hooks.
	for hookPoint, hook := range t.hooks {
		if err := hook.Close(); err != nil {
			log.Errorf("Failed to close '%s/%s': %v", hookPoint.group, hookPoint.name, err)
		}
		delete(t.hooks, hookPoint)
	}

	t.processManager.Close()
	t.signalDone()
}

// initializeMapsAndPrograms loads the definitions for the eBPF maps and programs provided
// by the embedded elf file and loads these into the kernel.
func initializeMapsAndPrograms(kmod *kallsyms.Module, cfg *Config) (
	ebpfMaps map[string]*cebpf.Map, ebpfProgs map[string]*cebpf.Program,
	stackdeltaInnerMapSpec *cebpf.MapSpec, err error,
) {
	// Loading specifications about eBPF programs and maps from the embedded elf file
	// does not load them into the kernel.
	// A collection specification holds the information about eBPF programs and maps.
	// References to eBPF maps in the eBPF programs are just placeholders that need to be
	// replaced by the actual loaded maps later on with rewriteMaps before loading the
	// programs into the kernel.
	major, minor, patch, err := linux.GetCurrentKernelVersion()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get kernel version: %v", err)
	}

	coll, err := support.LoadCollectionSpec()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load specification for tracers: %v", err)
	}

	if major > 6 || (major == 6 && minor >= 16) {
		// Tracepoint format for sched_process_free has changed in v6.16+.
		delete(coll.Programs, schedProcessFreeV1)
	} else {
		delete(coll.Programs, schedProcessFreeV2)
	}

	// Initialize eBPF variables before loading programs and maps.
	if err = loadRodataVars(coll, kmod, cfg); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to set RODATA variables: %v", err)
	}

	ebpfMaps = make(map[string]*cebpf.Map)
	ebpfProgs = make(map[string]*cebpf.Program)

	// Use the specification of the first exe_id_to_Y_stack_deltas inner map
	// as template for further updates.
	innerMapTemplate := coll.Maps["exe_id_to_8_stack_deltas"].InnerMap.Copy()

	// Since we load maps individually with cebpf.NewMap, we must manually propagate
	// variable values back into the map spec contents before creating the maps.
	if err = syncVariablesToMapSpecs(coll); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to sync variables to map specs: %v", err)
	}

	// Load all maps into the kernel that are used later on in eBPF programs. So we can rewrite
	// in the next step the placesholders in the eBPF programs with the file descriptors of the
	// loaded maps in the kernel.
	if err = loadAllMaps(coll, cfg, ebpfMaps); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load eBPF maps: %v", err)
	}

	// Replace the place holders for map access in the eBPF programs with
	// the file descriptors of the loaded maps.
	if err = rewriteMaps(coll, ebpfMaps); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to rewrite maps: %v", err)
	}

	if cfg.KernelVersionCheck {
		if hasProbeReadBug(major, minor, patch) {
			if err = checkForMaccessPatch(coll, ebpfMaps, kmod); err != nil {
				return nil, nil, nil, fmt.Errorf("your kernel version %d.%d.%d may be "+
					"affected by a Linux kernel bug that can lead to system "+
					"freezes, terminating host agent now to avoid "+
					"triggering this bug.\n"+
					"If you are certain your kernel is not affected, "+
					"you can override this check at your own risk "+
					"with -no-kernel-version-check.\n"+
					"Error: %v", major, minor, patch, err)
			}
		}
	}

	tailCallProgs := []progLoaderHelper{
		{
			progID: uint32(support.ProgUnwindStop),
			name:   "unwind_stop",
			enable: true,
		},
		{
			progID: uint32(support.ProgUnwindNative),
			name:   "unwind_native",
			enable: true,
		},
		{
			progID: uint32(support.ProgUnwindHotspot),
			name:   "unwind_hotspot",
			enable: cfg.IncludeTracers.Has(tracertypes.HotspotTracer),
		},
		{
			progID: uint32(support.ProgUnwindPerl),
			name:   "unwind_perl",
			enable: cfg.IncludeTracers.Has(tracertypes.PerlTracer),
		},
		{
			progID: uint32(support.ProgUnwindPHP),
			name:   "unwind_php",
			enable: cfg.IncludeTracers.Has(tracertypes.PHPTracer),
		},
		{
			progID: uint32(support.ProgUnwindPython),
			name:   "unwind_python",
			enable: cfg.IncludeTracers.Has(tracertypes.PythonTracer),
		},
		{
			progID: uint32(support.ProgUnwindRuby),
			name:   "unwind_ruby",
			enable: cfg.IncludeTracers.Has(tracertypes.RubyTracer),
		},
		{
			progID: uint32(support.ProgUnwindV8),
			name:   "unwind_v8",
			enable: cfg.IncludeTracers.Has(tracertypes.V8Tracer),
		},
		{
			progID: uint32(support.ProgUnwindDotnet),
			name:   "unwind_dotnet",
			enable: cfg.IncludeTracers.Has(tracertypes.DotnetTracer),
		},
		{
			progID: uint32(support.ProgUnwindDotnet10),
			name:   "unwind_dotnet10",
			enable: cfg.IncludeTracers.Has(tracertypes.DotnetTracer),
		},
		{
			progID: uint32(support.ProgGoLabels),
			name:   "go_labels",
			enable: cfg.IncludeTracers.Has(tracertypes.Labels),
		},
		{
			progID: uint32(support.ProgUnwindBEAM),
			name:   "unwind_beam",
			enable: cfg.IncludeTracers.Has(tracertypes.BEAMTracer),
		},
	}

	if err = loadPerfUnwinders(coll, ebpfProgs, ebpfMaps["perf_progs"], tailCallProgs,
		cfg.BPFVerifierLogLevel); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load perf eBPF programs: %v", err)
	}

	if cfg.OffCPUThreshold > 0 || len(cfg.ProbeLinks) > 0 || cfg.LoadProbe {
		// Load the tail call destinations if any kind of event profiling is enabled.
		if err = loadProbeUnwinders(coll, ebpfProgs, ebpfMaps["kprobe_progs"], tailCallProgs,
			cfg.BPFVerifierLogLevel, ebpfMaps["perf_progs"].FD()); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load kprobe eBPF programs: %v", err)
		}
	}

	if cfg.OffCPUThreshold > 0 {
		offCPUProgs := []progLoaderHelper{
			{
				name:             "finish_task_switch",
				noTailCallTarget: true,
				enable:           true,
			},
			{
				name:             "tracepoint__sched_switch",
				noTailCallTarget: true,
				enable:           true,
			},
		}
		if err = loadProbeUnwinders(coll, ebpfProgs, ebpfMaps["kprobe_progs"], offCPUProgs,
			cfg.BPFVerifierLogLevel, ebpfMaps["perf_progs"].FD()); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load kprobe eBPF programs: %v", err)
		}
	}

	if len(cfg.ProbeLinks) > 0 || cfg.LoadProbe {
		probeProgs := []progLoaderHelper{
			{
				name:             genericProgName,
				noTailCallTarget: true,
				enable:           true,
			},
		}
		if err = loadProbeUnwinders(coll, ebpfProgs, ebpfMaps["kprobe_progs"], probeProgs,
			cfg.BPFVerifierLogLevel, ebpfMaps["perf_progs"].FD()); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load uprobe eBPF programs: %v", err)
		}
	}

	if err = loadKallsymsTrigger(coll, ebpfProgs, cfg.BPFVerifierLogLevel); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load kallsym eBPF program: %v", err)
	}

	if err = removeTemporaryMaps(ebpfMaps); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to remove temporary maps: %v", err)
	}

	return ebpfMaps, ebpfProgs, innerMapTemplate, nil
}

// removeTemporaryMaps unloads and deletes eBPF maps that are only required for the
// initialization.
func removeTemporaryMaps(ebpfMaps map[string]*cebpf.Map) error {
	for _, mapName := range []string{"system_analysis"} {
		if err := ebpfMaps[mapName].Close(); err != nil {
			log.Errorf("Failed to close %s: %v", mapName, err)
			return err
		}
		delete(ebpfMaps, mapName)
	}
	return nil
}

// rewriteMaps replaces all references to named eBPF maps.
// This means that pre-existing maps are used, instead of new ones created
// when calling NewCollection. Any named eBPF maps are removed from CollectionSpec.Maps.
//
// This function used to be part of cilium/ebpf before it got deprecated and finally removed.
// As we require to resize maps in loadAllMaps() we still need this functionality.
// cilium/ebpf deprecated this function as FDs from *cebpf.Map are taken, but no references to
// the Map entries are kept. If no one else holds a reference to these Map entries, they can
// get GCd, the FD closed and the program load fails.
//
// Returns an error if a named eBPF map isn't used in at least one program.
func rewriteMaps(coll *cebpf.CollectionSpec, maps map[string]*cebpf.Map) error {
	for symbol, m := range maps {
		// have we seen a program that uses this symbol / map
		seen := false
		for progName, progSpec := range coll.Programs {
			err := progSpec.Instructions.AssociateMap(symbol, m)

			switch {
			case err == nil:
				seen = true

			case errors.Is(err, asm.ErrUnreferencedSymbol):
				// Not all programs need to use the map

			default:
				return fmt.Errorf("program %s: %w", progName, err)
			}
		}

		if !seen {
			return fmt.Errorf("map %s not referenced by any programs", symbol)
		}

		// Prevent NewCollection from creating rewritten maps
		delete(coll.Maps, symbol)
	}

	return nil
}

// syncVariablesToMapSpecs copies VariableSpec values back into the corresponding
// MapSpec contents. This is necessary starting from cilium/ebpf v0.21.0, as
// VariableSpec.Set() only updates VariableSpec.Value and no longer directly
// modifies the underlying MapSpec.Contents byte slice.
func syncVariablesToMapSpecs(coll *cebpf.CollectionSpec) error {
	for mapName, mapSpec := range coll.Maps {
		if len(mapSpec.Contents) == 0 {
			continue
		}
		data, ok := mapSpec.Contents[0].Value.([]byte)
		if !ok {
			continue
		}
		modified := false
		for _, v := range coll.Variables {
			if v.SectionName != mapName || len(v.Value) == 0 {
				continue
			}
			end := int(v.Offset) + len(v.Value)
			if end > len(data) {
				return fmt.Errorf("variable %s (offset %d, size %d) exceeds map %s data size %d",
					v.Name, v.Offset, len(v.Value), mapName, len(data))
			}
			if !modified {
				// Clone the underlying slice to avoid modifying shared data
				// (MapSpec.Copy performs a shallow copy of Contents).
				data = slices.Clone(data)
				modified = true
			}
			copy(data[v.Offset:end], v.Value)
		}
		if modified {
			mapSpec.Contents[0] = cebpf.MapKV{Key: uint32(0), Value: data}
		}
	}
	return nil
}

// loadAllMaps loads all eBPF maps that are used in our eBPF programs.
func loadAllMaps(coll *cebpf.CollectionSpec, cfg *Config,
	ebpfMaps map[string]*cebpf.Map,
) error {
	restoreRlimit, err := rlimit.MaximizeMemlock()
	if err != nil {
		return fmt.Errorf("failed to adjust rlimit: %v", err)
	}
	defer restoreRlimit()

	// Redefine the maximum number of map entries for selected eBPF maps.
	adaption := make(map[string]uint32, 4)

	const (
		// The following sizes X are used as 2^X, and determined empirically.
		// 1 million executable pages / 4GB of executable address space
		pidPageMappingInfoSize   = 20
		stackDeltaPageToInfoSize = 16
		exeIDToStackDeltasSize   = 16
	)

	adaption["pid_page_to_mapping_info"] = 1 << uint32(pidPageMappingInfoSize+cfg.MapScaleFactor)

	adaption["stack_delta_page_to_info"] = 1 << uint32(stackDeltaPageToInfoSize+cfg.MapScaleFactor)

	adaption["sched_times"] = schedTimesSize(cfg.OffCPUThreshold)

	for i := support.StackDeltaBucketSmallest; i <= support.StackDeltaBucketLargest; i++ {
		mapName := fmt.Sprintf("exe_id_to_%d_stack_deltas", i)
		adaption[mapName] = 1 << uint32(exeIDToStackDeltasSize+cfg.MapScaleFactor)
	}

	for mapName, mapSpec := range coll.Maps {
		if mapName == "sched_times" && cfg.OffCPUThreshold == 0 {
			// Off CPU Profiling is disabled. So do not load this map.
			continue
		}

		if !tracertypes.IsMapEnabled(mapName, cfg.IncludeTracers) {
			log.Debugf("Skipping eBPF map %s: tracer not enabled", mapName)
			continue
		}
		if newSize, ok := adaption[mapName]; ok {
			log.Debugf("Size of eBPF map %s: %v", mapName, newSize)
			mapSpec.MaxEntries = newSize
		}
		ebpfMap, err := cebpf.NewMap(mapSpec)
		if err != nil {
			return fmt.Errorf("failed to load %s: %v", mapName, err)
		}
		ebpfMaps[mapName] = ebpfMap
	}

	return nil
}

// loadKallsymsTrigger loads the eBPF program that triggers kallsym updates.
func loadKallsymsTrigger(coll *cebpf.CollectionSpec,
	ebpfProgs map[string]*cebpf.Program, bpfVerifierLogLevel uint32) error {
	programOptions := cebpf.ProgramOptions{
		LogLevel: cebpf.LogLevel(bpfVerifierLogLevel),
	}

	kallsymsTriggerProg := "kprobe__kallsyms"
	progSpec, ok := coll.Programs[kallsymsTriggerProg]
	if !ok {
		return fmt.Errorf("program %s does not exist", kallsymsTriggerProg)
	}

	if err := loadProgram(ebpfProgs, nil, 0, progSpec,
		programOptions, true); err != nil {
		return err
	}

	return nil
}

// schedTimesSize calculates the size of the sched_times map based on the
// configured off-cpu threshold.
// To not lose too many scheduling events but also not oversize sched_times,
// calculate a size based on an assumed upper bound of scheduler events per
// second (1000hz) multiplied by an average time a task remains off CPU (3s),
// scaled by the probability of capturing a trace.
func schedTimesSize(threshold uint32) uint32 {
	size := uint32((4096 * uint64(threshold)) / math.MaxUint32)
	if size < 16 {
		// Guarantee a minimal size of 16.
		return 16
	}
	if size > 4096 {
		// Guarantee a maximum size of 4096.
		return 4096
	}
	return size
}

// loadPerfUnwinders loads all perf eBPF Programs and their tail call targets.
func loadPerfUnwinders(coll *cebpf.CollectionSpec, ebpfProgs map[string]*cebpf.Program,
	tailcallMap *cebpf.Map, tailCallProgs []progLoaderHelper,
	bpfVerifierLogLevel uint32,
) error {
	programOptions := cebpf.ProgramOptions{
		LogLevel: cebpf.LogLevel(bpfVerifierLogLevel),
	}

	progs := make([]progLoaderHelper, len(tailCallProgs)+2)
	copy(progs, tailCallProgs)

	schedProcessFree := schedProcessFreeHookName(libpf.MapKeysToSet(coll.Programs))
	progs = append(progs,
		progLoaderHelper{
			name:             schedProcessFree,
			noTailCallTarget: true,
			enable:           true,
		},
		progLoaderHelper{
			name:             "native_tracer_entry",
			noTailCallTarget: true,
			enable:           true,
		})

	for _, unwindProg := range progs {
		if !unwindProg.enable {
			continue
		}

		unwindProgName := unwindProg.name
		if !unwindProg.noTailCallTarget {
			unwindProgName = "perf_" + unwindProg.name
		}

		progSpec, ok := coll.Programs[unwindProgName]
		if !ok {
			return fmt.Errorf("program %s does not exist", unwindProgName)
		}

		if err := loadProgram(ebpfProgs, tailcallMap, unwindProg.progID, progSpec,
			programOptions, unwindProg.noTailCallTarget); err != nil {
			return err
		}
	}

	return nil
}

// progArrayReferences returns a list of instructions which load a specified tail
// call FD.
func progArrayReferences(perfTailCallMapFD int, insns asm.Instructions) []int {
	insNos := []int{}
	for i := range insns {
		ins := &insns[i]
		if asm.OpCode(ins.OpCode.Class()) != asm.OpCode(asm.LdClass) {
			continue
		}
		m := ins.Map()
		if m == nil {
			continue
		}
		if perfTailCallMapFD == m.FD() {
			insNos = append(insNos, i)
		}
	}
	return insNos
}

// loadProbeUnwinders reuses large parts of loadPerfUnwinders. By default all eBPF programs
// are written as perf event eBPF programs. loadProbeUnwinders dynamically rewrites the
// specification of these programs to xProbe eBPF programs and adjusts tail call maps.
func loadProbeUnwinders(coll *cebpf.CollectionSpec, ebpfProgs map[string]*cebpf.Program,
	tailcallMap *cebpf.Map, progs []progLoaderHelper,
	bpfVerifierLogLevel uint32, perfTailCallMapFD int,
) error {
	programOptions := cebpf.ProgramOptions{
		LogLevel: cebpf.LogLevel(bpfVerifierLogLevel),
	}

	for _, unwindProg := range progs {
		if !unwindProg.enable {
			continue
		}

		unwindProgName := unwindProg.name
		if !unwindProg.noTailCallTarget {
			unwindProgName = "kprobe_" + unwindProg.name
		}

		progSpec, ok := coll.Programs[unwindProgName]
		if !ok {
			return fmt.Errorf("program %s does not exist", unwindProgName)
		}

		// Replace the prog array for the tail calls.
		insns := progArrayReferences(perfTailCallMapFD, progSpec.Instructions)
		for _, ins := range insns {
			if err := progSpec.Instructions[ins].AssociateMap(tailcallMap); err != nil {
				return fmt.Errorf("failed to rewrite map ptr: %v", err)
			}
		}

		if err := loadProgram(ebpfProgs, tailcallMap, unwindProg.progID, progSpec,
			programOptions, unwindProg.noTailCallTarget); err != nil {
			return err
		}
	}

	return nil
}

// loadProgram loads an eBPF program from progSpec and populates the related maps.
func loadProgram(ebpfProgs map[string]*cebpf.Program, tailcallMap *cebpf.Map,
	progID uint32, progSpec *cebpf.ProgramSpec, programOptions cebpf.ProgramOptions,
	noTailCallTarget bool,
) error {
	restoreRlimit, err := rlimit.MaximizeMemlock()
	if err != nil {
		return fmt.Errorf("failed to adjust rlimit: %v", err)
	}
	defer restoreRlimit()

	// Load the eBPF program into the kernel. If no error is returned,
	// the eBPF program can be used/called/triggered from now on.
	unwinder, err := cebpf.NewProgramWithOptions(progSpec, programOptions)
	if err != nil {
		// These errors tend to have hundreds of lines (or more),
		// so we print each line individually.
		if ve, ok := err.(*cebpf.VerifierError); ok {
			for _, line := range ve.Log {
				log.Errorf("%s", line)
			}
		} else {
			scanner := bufio.NewScanner(strings.NewReader(err.Error()))
			for scanner.Scan() {
				log.Errorf("%s", scanner.Text())
			}
		}
		return fmt.Errorf("failed to load %s", progSpec.Name)
	}
	ebpfProgs[progSpec.Name] = unwinder

	if noTailCallTarget {
		return nil
	}
	fd := uint32(unwinder.FD())
	if err := tailcallMap.Update(unsafe.Pointer(&progID), unsafe.Pointer(&fd),
		cebpf.UpdateAny); err != nil {
		// Every eBPF program that is loaded within loadUnwinders can be the
		// destination of a tail call of another eBPF program. If we can not update
		// the eBPF map that manages these destinations our unwinding will fail.
		return fmt.Errorf("failed to update tailcall map: %v", err)
	}
	return nil
}

// readKernelFrames fetches the kernel stack frames for a particular kstackID and
// returns them as symbolized libpf.Frames.
func (t *Tracer) readKernelFrames(kstackID int32, oldFrames libpf.Frames) (libpf.Frames, error) {
	cKstackID := kstackID
	kstackVal := make([]uint64, support.PerfMaxStackDepth)

	if err := t.ebpfMaps["kernel_stackmap"].Lookup(unsafe.Pointer(&cKstackID),
		unsafe.Pointer(&kstackVal[0])); err != nil {
		return nil, fmt.Errorf("failed to lookup kernel frames for stackID %d: %v", kstackID, err)
	}

	// The kernel returns absolute addresses in kernel address
	// space format. Here just the stack length is needed.
	// But also debug print the symbolization based on kallsyms.
	var kstackLen uint32
	for kstackLen < support.PerfMaxStackDepth && kstackVal[kstackLen] != 0 {
		kstackLen++
	}

	frames := oldFrames
	if kstackLen > uint32(len(frames)) {
		frames = make(libpf.Frames, 0, kstackLen)
	}
	for i := uint32(0); i < kstackLen; i++ {
		address := libpf.Address(kstackVal[i])
		frame := libpf.Frame{
			Type:            libpf.KernelFrame,
			AddressOrLineno: libpf.AddressOrLineno(address - 1),
		}

		kmod, err := t.kernelSymbolizer.GetModuleByAddress(address)
		if err == nil {
			frame.Mapping = kmod.Mapping()
			frame.AddressOrLineno -= libpf.AddressOrLineno(kmod.Start())

			if funcName, _, err := kmod.LookupSymbolByAddress(address); err == nil {
				frame.FunctionName = libpf.Intern(funcName)
			}
		}
		frames.Append(&frame)
	}

	return frames, nil
}

// enableEvent removes the entry of given eventType from the inhibitEvents map
// so that the eBPF code will send the event again.
func (t *Tracer) enableEvent(eventType int) {
	inhibitEventsMap := t.ebpfMaps["inhibit_events"]

	// The map entry might not exist, so just ignore the potential error.
	et := uint32(eventType)
	_ = inhibitEventsMap.Delete(unsafe.Pointer(&et))
}

// monitorPIDEventsMapSingle iterates over the eBPF map pid_events, collects PIDs
// and writes them to the keys slice.
func (t *Tracer) monitorPIDEventsMapSingle(keys *[]libpf.PIDTID) error {
	eventsMap := t.ebpfMaps["pid_events"]
	var key, nextKey uint64
	var value bool
	keyFound := true
	deleteBatch := make(libpf.Set[uint64])

	// Key 0 retrieves the very first element in the hash map as
	// it is guaranteed not to exist in pid_events.
	key = 0
	if err := eventsMap.NextKey(unsafe.Pointer(&key), unsafe.Pointer(&nextKey)); err != nil {
		if errors.Is(err, cebpf.ErrKeyNotExist) {
			return nil
		}
		return fmt.Errorf("Failed to read from pid_events map: %v", err)
	}

	for keyFound {
		key = nextKey

		if err := eventsMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&value)); err != nil {
			return fmt.Errorf("Failed to lookup '%v' in pid_events: %v", key, err)
		}

		// Lookup the next map entry before deleting the current one.
		if err := eventsMap.NextKey(unsafe.Pointer(&key), unsafe.Pointer(&nextKey)); err != nil {
			if !errors.Is(err, cebpf.ErrKeyNotExist) {
				return fmt.Errorf("Failed to read from pid_events map: %v", err)
			}
			keyFound = false
		}

		if !t.hasBatchOperations {
			// Now that we have the next key, we can delete the current one.
			if err := eventsMap.Delete(unsafe.Pointer(&key)); err != nil {
				return fmt.Errorf("Failed to delete '%v' from pid_events: %v", key, err)
			}
		} else {
			// Store to-be-deleted keys in a map so we can delete them all with a single
			// bpf syscall.
			deleteBatch[key] = libpf.Void{}
		}

		// If we process keys inline with iteration (e.g. by sending them to t.pidEvents at this
		// exact point), we may block sending to the channel, delay the iteration and may introduce
		// race conditions (related to deletion). For that reason, keys are first collected and,
		// after the iteration has finished, sent to the channel.
		*keys = append(*keys, libpf.PIDTID(key))
	}

	keysToDelete := len(deleteBatch)
	if keysToDelete != 0 {
		keys := libpf.MapKeysToSlice(deleteBatch)
		if _, err := eventsMap.BatchDelete(keys, nil); err != nil {
			return fmt.Errorf("Failed to batch delete %d entries from pid_events map: %v",
				keysToDelete, err)
		}
	}
	return nil
}

// monitorPIDEventsMapBatch iterates over the eBPF map pid_events in batches,
// collects PIDs and writes them to the keys slice.
func (t *Tracer) monitorPIDEventsMapBatch(keys *[]libpf.PIDTID) error {
	eventsMap := t.ebpfMaps["pid_events"]

	removed := make([]uint64, 128)
	values := make([]bool, len(removed))

	cursor := cebpf.MapBatchCursor{}

	for {
		n, err := eventsMap.BatchLookupAndDelete(&cursor, removed, values, nil)

		// There can be results even if there's an error.
		for i := range n {
			*keys = append(*keys, libpf.PIDTID(removed[i]))
		}

		if errors.Is(err, cebpf.ErrKeyNotExist) {
			break
		}

		if err != nil {
			return fmt.Errorf("Failed to batch lookup and delete entries from pid_events map: %v", err)
		}
	}

	return nil
}

// eBPFMetricsCollector retrieves the eBPF metrics, calculates their delta values,
// and translates eBPF IDs into Metric ID.
// Returns a slice of Metric ID/Value pairs.
func (t *Tracer) eBPFMetricsCollector(
	translateIDs []metrics.MetricID,
	previousMetricValue []metrics.MetricValue,
) []metrics.Metric {
	metricsMap := t.ebpfMaps["metrics"]
	metricsUpdates := make([]metrics.Metric, 0, len(translateIDs))

	// Iterate over all known metric IDs
	for ebpfID, metricID := range translateIDs {
		var perCPUValues []uint64

		// Checking for 'gaps' in the translation table.
		// That allows non-contiguous metric IDs, e.g. after removal/deprecation of a metric ID.
		if metricID == metrics.IDInvalid {
			continue
		}

		eID := uint32(ebpfID)
		if err := metricsMap.Lookup(unsafe.Pointer(&eID), &perCPUValues); err != nil {
			log.Errorf("Failed trying to lookup per CPU element: %v", err)
			continue
		}
		value := metrics.MetricValue(0)
		for _, val := range perCPUValues {
			value += metrics.MetricValue(val)
		}

		// The monitoring infrastructure expects instantaneous values (gauges).
		// => for cumulative metrics (counters), send deltas of the observed values, so they
		// can be interpreted as gauges.
		if ebpfID < support.MetricIDBeginCumulative {
			// We don't assume 64bit counters to overflow
			deltaValue := value - previousMetricValue[ebpfID]

			// 0 deltas add no value when summed up for display purposes in the UI
			if deltaValue == 0 {
				continue
			}

			previousMetricValue[ebpfID] = value
			value = deltaValue
		}

		// Collect the metrics for reporting
		metricsUpdates = append(metricsUpdates, metrics.Metric{
			ID:    metricID,
			Value: value,
		})
	}

	return metricsUpdates
}

// Various bpf trace handling related errors:
var (
	errRecordTooSmall       = errors.New("trace record too small")
	errRecordUnexpectedSize = errors.New("unexpected record size")
	errOriginUnexpected     = errors.New("unexpected origin")
)

// loadBpfTrace parses a raw BPF trace into a `host.Trace` instance.
//
// If the raw trace contains a kernel stack ID, the kernel stack is also
// retrieved and inserted at the appropriate position.
func (t *Tracer) loadBpfTrace(raw []byte, cpu int) (*libpf.EbpfTrace, error) {
	frameListOffs := int(unsafe.Offsetof(support.Trace{}.Frame_data))

	if len(raw) < frameListOffs {
		return nil, fmt.Errorf("%d < %d: %w", len(raw), frameListOffs, errRecordTooSmall)
	}

	ptr := traceFromRaw(raw)
	frameDataLen := int(ptr.Frame_data_len) * 8

	// NOTE: can't do exact check here: kernel adds a few padding bytes to messages.
	if len(raw) < frameListOffs+frameDataLen {
		return nil, fmt.Errorf("%d < %d: %w", len(raw), frameListOffs+frameDataLen,
			errRecordUnexpectedSize)
	}

	pid := libpf.PID(ptr.Pid)
	procMeta := t.processManager.MetaForPID(pid)
	trace := t.tracePool.Get().(*libpf.EbpfTrace)
	*trace = libpf.EbpfTrace{
		Comm:             goString(ptr.Comm[:]),
		ExecutablePath:   procMeta.Executable,
		ContainerID:      procMeta.ContainerID,
		ProcessName:      procMeta.Name,
		APMTraceID:       *(*libpf.APMTraceID)(unsafe.Pointer(&ptr.Apm_trace_id)),
		APMTransactionID: *(*libpf.APMTransactionID)(unsafe.Pointer(&ptr.Apm_transaction_id)),
		PID:              pid,
		TID:              libpf.PID(ptr.Tid),
		Origin:           libpf.Origin(ptr.Origin),
		OffTime:          int64(ptr.Offtime),
		KTime:            int64(ptr.Ktime),
		CPU:              cpu,
		EnvVars:          procMeta.EnvVariables,
	}

	switch trace.Origin {
	case support.TraceOriginSampling:
	case support.TraceOriginOffCPU:
	case support.TraceOriginProbe:
	default:
		return nil, fmt.Errorf("origin %d: %w", trace.Origin, errOriginUnexpected)
	}

	if ptr.Kernel_stack_id >= 0 {
		var err error
		trace.KernelFrames, err = t.readKernelFrames(ptr.Kernel_stack_id, trace.KernelFrames)
		if err != nil {
			log.Errorf("Failed to get kernel stack frames: %v", err)
		}
	}

	if ptr.Custom_labels.Len > 0 {
		trace.CustomLabels = make(map[libpf.String]libpf.String, int(ptr.Custom_labels.Len))
		for i := 0; i < int(ptr.Custom_labels.Len); i++ {
			lbl := ptr.Custom_labels.Labels[i]
			key := goString(lbl.Key[:])
			val := goString(lbl.Val[:])
			trace.CustomLabels[key] = val
		}
	}

	trace.NumFrames = int(ptr.Num_frames)
	trace.FrameData = trace.FrameDataBuf[:ptr.Frame_data_len]
	copy(trace.FrameData, ptr.Frame_data[:ptr.Frame_data_len])

	return trace, nil
}

// StartMapMonitors starts goroutines for collecting metrics and monitoring eBPF
// maps for tracepoints, new traces, trace count updates and unknown PCs.
func (t *Tracer) StartMapMonitors(ctx context.Context, traceOutChan chan<- *libpf.EbpfTrace) error {
	if err := t.kernelSymbolizer.StartMonitor(ctx); err != nil {
		log.Warnf("Failed to start kallsyms monitor: %v", err)
	}
	eventMetricCollector, err := t.startEventMonitor(ctx)
	if err != nil {
		return err
	}
	traceEventMetricCollector, err := t.startTraceEventMonitor(ctx, traceOutChan)
	if err != nil {
		return err
	}

	pidEvents := make([]libpf.PIDTID, 0)
	periodiccaller.StartWithManualTrigger(ctx, t.intervals.MonitorInterval(),
		t.triggerPIDProcessing, func(_ bool) bool {
			t.enableEvent(support.EventTypeGenericPID)
			err := t.monitorPIDEventsMapMethod(&pidEvents)
			if err != nil {
				log.Errorf("Failed to monitor PID events: %v", err)
				t.signalDone()
				return false
			}

			for _, pidTid := range pidEvents {
				log.Debugf("=> %v", pidTid)
				t.pidEvents <- pidTid
			}

			// Keep the underlying array alive to avoid GC pressure
			pidEvents = pidEvents[:0]
			return true
		})

	// translateIDs is a translation table for eBPF IDs into Metric IDs.
	// Index is the ebpfID, value is the corresponding metricID.
	translateIDs := support.MetricsTranslation

	// previousMetricValue stores the previously retrieved metric values to
	// calculate and store delta values.
	previousMetricValue := make([]metrics.MetricValue, len(translateIDs))

	periodiccaller.Start(ctx, t.intervals.MonitorInterval(), func() {
		metrics.AddSlice(eventMetricCollector())
		metrics.AddSlice(traceEventMetricCollector())
		metrics.AddSlice(t.eBPFMetricsCollector(translateIDs, previousMetricValue))
	})

	return nil
}

func (t *Tracer) attachToKallsymsUpdates() error {
	prog, ok := t.ebpfProgs["kprobe__kallsyms"]
	if !ok {
		return fmt.Errorf("kprobe__kallsyms is not available")
	}

	kallsymsAttachPoint := "bpf_ksym_add"
	kmod, err := t.kernelSymbolizer.GetModuleByName(kallsyms.Kernel)
	if err != nil {
		return err
	}

	if _, err := kmod.LookupSymbol(kallsymsAttachPoint); err != nil {
		log.Infof("Monitoring kallsyms is supported only for Linux kernel 5.8 or greater: %s: %v",
			kallsymsAttachPoint, err)
		return nil
	}

	hook, err := link.Kprobe(kallsymsAttachPoint, prog, nil)
	if err != nil {
		return fmt.Errorf("failed opening kprobe for kallsyms trigger: %s", err)
	}
	t.hooks[hookPoint{group: "kprobe", name: kallsymsAttachPoint}] = hook

	return nil
}

// terminatePerfEvents disables perf events and closes their file descriptor.
func terminatePerfEvents(events []*perf.Event) {
	for _, event := range events {
		if err := event.Disable(); err != nil {
			log.Errorf("Failed to disable perf event: %v", err)
		}
		if err := event.Close(); err != nil {
			log.Errorf("Failed to close perf event: %v", err)
		}
	}
}

// AttachTracer attaches the main tracer entry point to the perf interrupt events. The tracer
// entry point is always the native tracer. The native tracer will determine when to invoke the
// interpreter tracers based on address range information.
func (t *Tracer) AttachTracer() error {
	tracerProg, ok := t.ebpfProgs["native_tracer_entry"]
	if !ok {
		return errors.New("entry program is not available")
	}

	perfAttribute := new(perf.Attr)
	perfAttribute.SetSampleFreq(uint64(t.samplesPerSecond))
	if err := perf.CPUClock.Configure(perfAttribute); err != nil {
		return fmt.Errorf("failed to configure software perf event: %v", err)
	}

	onlineCPUIDs, err := getOnlineCPUIDs()
	if err != nil {
		return fmt.Errorf("failed to get online CPUs: %v", err)
	}

	events := t.perfEntrypoints.WLock()
	defer t.perfEntrypoints.WUnlock(&events)
	for _, id := range onlineCPUIDs {
		perfEvent, err := perf.Open(perfAttribute, perf.AllThreads, id, nil)
		if err != nil {
			terminatePerfEvents(*events)
			return fmt.Errorf("failed to attach to perf event on CPU %d: %v", id, err)
		}
		if err := perfEvent.SetBPF(uint32(tracerProg.FD())); err != nil {
			terminatePerfEvents(*events)
			return fmt.Errorf("failed to attach eBPF program to perf event: %v", err)
		}
		*events = append(*events, perfEvent)
	}

	if err = t.attachToKallsymsUpdates(); err != nil {
		terminatePerfEvents(*events)
		return err
	}

	return nil
}

// EnableProfiling enables the perf interrupt events with the attached eBPF programs.
func (t *Tracer) EnableProfiling() error {
	events := t.perfEntrypoints.WLock()
	defer t.perfEntrypoints.WUnlock(&events)
	if len(*events) == 0 {
		return errors.New("no perf events available to enable for profiling")
	}
	for id, event := range *events {
		if err := event.Enable(); err != nil {
			return fmt.Errorf("failed to enable perf event on CPU %d: %v", id, err)
		}
	}
	return nil
}

// probabilisticProfile performs a single iteration of probabilistic profiling. It will generate
// a random number between 0 and ProbabilisticThresholdMax-1 every interval. If the random
// number is smaller than threshold it will enable the frequency based sampling for this
// time interval. Otherwise the frequency based sampling events are disabled.
func (t *Tracer) probabilisticProfile(interval time.Duration, threshold uint) {
	enableSampling := false
	probProfilingStatus := probProfilingDisable

	//nolint:gosec
	if rand.UintN(ProbabilisticThresholdMax) < threshold {
		enableSampling = true
		probProfilingStatus = probProfilingEnable
		log.Debugf("Start sampling for next interval (%v)", interval)
	} else {
		log.Debugf("Stop sampling for next interval (%v)", interval)
	}

	events := t.perfEntrypoints.WLock()
	defer t.perfEntrypoints.WUnlock(&events)
	var enableErr, disableErr metrics.MetricValue
	for _, event := range *events {
		if enableSampling {
			if err := event.Enable(); err != nil {
				enableErr++
				log.Errorf("Failed to enable frequency based sampling: %v",
					err)
			}
			continue
		}
		if err := event.Disable(); err != nil {
			disableErr++
			log.Errorf("Failed to disable frequency based sampling: %v", err)
		}
	}
	if enableErr != 0 {
		metrics.Add(metrics.IDPerfEventEnableErr, enableErr)
	}
	if disableErr != 0 {
		metrics.Add(metrics.IDPerfEventDisableErr, disableErr)
	}
	metrics.Add(metrics.IDProbProfilingStatus,
		metrics.MetricValue(probProfilingStatus))
}

// StartProbabilisticProfiling periodically runs probabilistic profiling.
func (t *Tracer) StartProbabilisticProfiling(ctx context.Context) {
	metrics.Add(metrics.IDProbProfilingInterval,
		metrics.MetricValue(t.probabilisticInterval.Seconds()))

	// Run a single iteration of probabilistic profiling to avoid needing
	// to wait for the first interval to pass with periodiccaller.Start()
	// before getting called.
	t.probabilisticProfile(t.probabilisticInterval, t.probabilisticThreshold)

	periodiccaller.Start(ctx, t.probabilisticInterval, func() {
		t.probabilisticProfile(t.probabilisticInterval, t.probabilisticThreshold)
	})
}

// StartOffCPUProfiling starts off-cpu profiling by attaching the programs to the hooks.
func (t *Tracer) StartOffCPUProfiling() error {
	// Attach the second hook for off-cpu profiling first.
	kprobeProg, ok := t.ebpfProgs["finish_task_switch"]
	if !ok {
		return errors.New("off-cpu program finish_task_switch is not available")
	}

	kmod, err := t.kernelSymbolizer.GetModuleByName(kallsyms.Kernel)
	if err != nil {
		return err
	}

	hookSymbolPrefix := "finish_task_switch"
	kprobeSymbs := kmod.LookupSymbolsByPrefix(hookSymbolPrefix)
	if len(kprobeSymbs) == 0 {
		return errors.New("no finish_task_switch symbols found")
	}

	attached := false
	// Attach to all symbols with the prefix finish_task_switch.
	for _, symb := range kprobeSymbs {
		kprobeLink, linkErr := link.Kprobe(string(symb.Name), kprobeProg, nil)
		if linkErr != nil {
			log.Warnf("Failed to attach to %s: %v", symb.Name, linkErr)
			continue
		}
		attached = true
		t.hooks[hookPoint{group: "kprobe", name: string(symb.Name)}] = kprobeLink
	}
	if !attached {
		return fmt.Errorf("failed to attach to one of %d symbols with prefix '%s'",
			len(kprobeSymbs), hookSymbolPrefix)
	}

	// Attach the first hook that enables off-cpu profiling.
	tpProg, ok := t.ebpfProgs["tracepoint__sched_switch"]
	if !ok {
		return errors.New("tracepoint__sched_switch is not available")
	}
	tpLink, err := link.Tracepoint("sched", "sched_switch", tpProg, nil)
	if err != nil {
		return nil
	}
	t.hooks[hookPoint{group: "sched", name: "sched_switch"}] = tpLink

	return nil
}

func (t *Tracer) AttachProbes(probes []string) error {
	for _, probeStr := range probes {
		probeSpec, err := ParseProbe(probeStr)
		if err != nil {
			return err
		}

		uProbeProg, ok := t.ebpfProgs[probeSpec.ProgName]
		if !ok {
			return fmt.Errorf("%s is not available", probeSpec.ProgName)
		}

		probeLink, err := AttachProbe(uProbeProg, probeSpec)
		if err != nil {
			return err
		}

		t.hooks[hookPoint{group: probeSpec.Type.String(), name: probeStr}] = probeLink
	}
	return nil
}

func (t *Tracer) HandleTrace(bpfTrace *libpf.EbpfTrace) {
	t.processManager.HandleTrace(bpfTrace)

	// Reclain the EbpfTrace
	bpfTrace.KernelFrames = bpfTrace.KernelFrames[0:0]
	t.tracePool.Put(bpfTrace)
}
