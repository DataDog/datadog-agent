// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && linux

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/seccomp-tracer-kern.c pkg/ebpf/bytecode/build/runtime/seccomp-tracer.c pkg/ebpf/c

// Package seccomptracer is the system-probe side of the Seccomp Tracer check
package seccomptracer

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/seccomptracer/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	eventsMapName      = "seccomp_events"
	stackTracesMapName = "stack_traces"
	ringBufferPages    = 32
	channelSize        = 1024
	moduleName         = "seccomp_tracer"
	stackCleanupTTL    = 10 * time.Second
)

// seccompStatsKey represents the key of a single seccomp event
type seccompStatsKey struct {
	Pid           uint32 `json:"pid"`
	SyscallNr     uint32 `json:"syscallNr"`
	SeccompAction uint32 `json:"seccompAction"`
}

// seccompStatsValue holds the aggregated data for a single key
type seccompStatsValue struct {
	cgroupName    string
	commName      string
	stackTraces   map[int32]*model.StackTraceInfo // map of stack ID to stack trace info
	droppedStacks uint64
}

// Tracer is the eBPF side of the Seccomp Tracer check
type Tracer struct {
	m            *manager.Manager
	eventHandler *perf.EventHandler

	// Stack trace support
	stackTraceMap     *ebpf.Map
	stackIDLastSeen   map[int32]time.Time
	stackIDMutex      sync.Mutex
	mapCleaner        *ddebpf.MapCleaner[int32, [127]uint64]
	maxStacksPerTuple int

	// In-memory aggregation of events
	statsMu sync.Mutex
	stats   map[seccompStatsKey]*seccompStatsValue
}

func IsSupported(cfg *ddebpf.Config) (bool, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return false, fmt.Errorf("error detecting kernel version: %w", err)
	}
	// Ring buffers require kernel 5.8+
	if kv < kernel.VersionCode(5, 8, 0) {
		return false, fmt.Errorf("detected kernel version %s, but seccomp-tracer probe requires a kernel version of at least 5.8.0 (for ring buffers)", kv)
	}

	return cfg.EnableCORE, nil
}

// NewTracer creates a [Tracer]
// Note: Seccomp tracer requires CO-RE (uses bpf_task_pt_regs and bpf_get_current_task_btf)
func NewTracer(cfg *ddebpf.Config) (*Tracer, error) {
	isSupported, err := IsSupported(cfg)
	if err != nil {
		return nil, err
	}
	if !isSupported {
		return nil, fmt.Errorf("seccomp tracer is not supported on this kernel version")
	}

	return loadSeccompTracerProbe(cfg)
}

func startSeccompTracerProbe(buf bytecode.AssetReader, managerOptions manager.Options, cfg *ddebpf.Config) (*Tracer, error) {
	// Read configuration from system-probe config
	maxStacksPerTuple := pkgconfigsetup.SystemProbe().GetInt("seccomp_tracer.max_stacks_per_tuple")
	if maxStacksPerTuple <= 0 {
		maxStacksPerTuple = 10 // fallback to default
	}

	stackTracesEnabled := pkgconfigsetup.SystemProbe().GetBool("seccomp_tracer.stack_traces_enabled")
	log.Infof("Seccomp tracer: stack_traces_enabled=%v, max_stacks_per_tuple=%d", stackTracesEnabled, maxStacksPerTuple)

	t := &Tracer{
		stats:             make(map[seccompStatsKey]*seccompStatsValue),
		stackIDLastSeen:   make(map[int32]time.Time),
		maxStacksPerTuple: maxStacksPerTuple,
	}

	// Create event handler for ring buffer
	eventHandler, err := perf.NewEventHandler(
		eventsMapName,
		t.handleEvent,
		perf.UseRingBuffers(ringBufferPages*os.Getpagesize(), channelSize),
		perf.SendTelemetry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event handler: %w", err)
	}
	t.eventHandler = eventHandler

	// Set BPF constant for stack traces enabled
	stackTracesEnabledValue := uint64(0)
	if stackTracesEnabled {
		stackTracesEnabledValue = uint64(1)
	}

	managerOptions.ConstantEditors = append(managerOptions.ConstantEditors, manager.ConstantEditor{
		Name:  "stack_traces_enabled",
		Value: stackTracesEnabledValue,
	})

	// Create manager with event handler as a modifier
	m := ddebpf.NewManagerWithDefault(&manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kretprobe__seccomp_run_filters",
					UID:          "seccomp_tracer",
				},
			},
		},
	}, moduleName, eventHandler, &ebpftelemetry.ErrorsTelemetryModifier{})

	if err := m.InitWithOptions(buf, &managerOptions); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	ddebpf.AddProbeFDMappings(m.Manager)
	ddebpf.AddNameMappings(m.Manager, moduleName)

	t.m = m.Manager

	// Get reference to stack traces map
	stackTraceMap, _, err := m.GetMap(stackTracesMapName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stack traces map: %w", err)
	}
	t.stackTraceMap = stackTraceMap

	// Create map cleaner for stack traces
	mapCleaner, err := ddebpf.NewMapCleaner[int32, [127]uint64](stackTraceMap, 100, stackTracesMapName, moduleName)
	if err != nil {
		return nil, fmt.Errorf("failed to create map cleaner: %w", err)
	}
	t.mapCleaner = mapCleaner

	// Start the map cleaner
	mapCleaner.Start(stackCleanupTTL, nil, nil, func(nowTS int64, stackID int32, _ [127]uint64) bool {
		t.stackIDMutex.Lock()
		defer t.stackIDMutex.Unlock()

		lastSeen, exists := t.stackIDLastSeen[stackID]
		if !exists {
			// Stack ID not tracked, should not be cleaned up
			return false
		}

		// Clean up if last seen more than TTL ago
		if time.Since(lastSeen) > stackCleanupTTL {
			delete(t.stackIDLastSeen, stackID)
			return true
		}

		return false
	})

	return t, nil
}

// handleEvent processes a single seccomp event from the ring buffer
func (t *Tracer) handleEvent(data []byte) {
	if len(data) < int(unsafe.Sizeof(SeccompEvent{})) {
		log.Warnf("seccomp event data too short: %d bytes", len(data))
		return
	}

	// Parse the event
	event := (*SeccompEvent)(unsafe.Pointer(&data[0]))

	cgroupName := unix.ByteSliceToString(event.Cgroup[:])
	// Convert []int8 to []byte for comm
	commBytes := make([]byte, len(event.Comm))
	for i, b := range event.Comm {
		commBytes[i] = byte(b)
	}
	commName := unix.ByteSliceToString(commBytes)

	key := seccompStatsKey{
		Pid:           event.Pid,
		SyscallNr:     event.Syscall_nr,
		SeccompAction: event.Action,
	}

	stackID := event.Stack_id

	// Aggregate in memory
	t.statsMu.Lock()
	defer t.statsMu.Unlock()

	// Initialize the value for this key if needed
	value, exists := t.stats[key]
	if !exists {
		value = &seccompStatsValue{
			cgroupName:  cgroupName,
			commName:    commName,
			stackTraces: make(map[int32]*model.StackTraceInfo),
		}
		t.stats[key] = value
	}

	// Handle stack trace if present
	if stackID >= 0 {
		// Update last seen time for this stack ID
		t.stackIDMutex.Lock()
		t.stackIDLastSeen[stackID] = time.Now()
		t.stackIDMutex.Unlock()

		// Check if we already have this stack trace
		if trace, exists := value.stackTraces[stackID]; exists {
			// Already tracked, just increment count
			trace.Count++
		} else {
			// New stack trace - check if we've hit the limit
			if len(value.stackTraces) >= t.maxStacksPerTuple {
				// At the limit, drop it
				value.droppedStacks++
				return
			}

			// Read the stack trace from the BPF map
			var addressArray [127]uint64
			if err := t.stackTraceMap.Lookup(&stackID, &addressArray); err != nil {
				log.Debugf("failed to lookup stack trace %d: %v", stackID, err)
				// Still track it with empty addresses
				value.stackTraces[stackID] = &model.StackTraceInfo{
					StackID: stackID,
					Count:   1,
				}
				return
			}

			// Convert fixed-size array to slice, trimming trailing zeros
			addresses := make([]uint64, 0, 127)
			for _, addr := range addressArray {
				if addr == 0 {
					break
				}
				addresses = append(addresses, addr)
			}

			// Store the new stack trace
			value.stackTraces[stackID] = &model.StackTraceInfo{
				StackID:   stackID,
				Count:     1,
				Addresses: addresses,
			}
		}
	} else {
		// No stack trace captured - track with -1 as the key
		if trace, exists := value.stackTraces[-1]; exists {
			trace.Count++
		} else {
			value.stackTraces[-1] = &model.StackTraceInfo{
				StackID: -1,
				Count:   1,
			}
		}
	}
}

// Close releases all associated resources
func (t *Tracer) Close() {
	if t.mapCleaner != nil {
		t.mapCleaner.Stop()
	}
	ddebpf.RemoveNameMappings(t.m)
	if err := t.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping Seccomp Tracer: %s", err)
	}
}

// GetAndFlush gets the aggregated stats and clears them
func (t *Tracer) GetAndFlush() model.SeccompStats {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()

	result := make(model.SeccompStats, 0, len(t.stats))

	for key, value := range t.stats {
		// Calculate total count across all stacks
		var totalCount uint64
		stackTraces := make([]model.StackTraceInfo, 0, len(value.stackTraces))

		for _, trace := range value.stackTraces {
			totalCount += trace.Count
			// Copy the trace (dereference the pointer)
			stackTraces = append(stackTraces, *trace)
		}

		entry := model.SeccompStatsEntry{
			CgroupName:    value.cgroupName,
			SyscallNr:     key.SyscallNr,
			SeccompAction: key.SeccompAction,
			Pid:           key.Pid,
			Comm:          value.commName,
			Count:         totalCount,
			StackTraces:   stackTraces,
			DroppedStacks: value.droppedStacks,
		}

		result = append(result, entry)
	}

	// Clear the stats
	t.stats = make(map[seccompStatsKey]*seccompStatsValue)

	return result
}

func loadSeccompTracerProbe(cfg *ddebpf.Config) (*Tracer, error) {

	filename := "seccomp-tracer.o"
	if cfg.BPFDebug {
		log.Infof("Using debug version of seccomp-tracer probe")
		filename = "seccomp-tracer-debug.o"
	}

	var probe *Tracer
	err := ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		var err error
		probe, err = startSeccompTracerProbe(buf, opts, cfg)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of seccomp-tracer probe with ring buffer")
	return probe, nil
}
