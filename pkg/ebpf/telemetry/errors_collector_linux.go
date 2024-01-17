// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"sync"
	"syscall"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxErrno    = 64
	maxErrnoStr = "other"

	ebpfMapTelemetryNS    = "ebpf_maps"
	ebpfHelperTelemetryNS = "ebpf_helpers"
)

const (
	readIndx int = iota
	readUserIndx
	readKernelIndx
	skbLoadBytes
	perfEventOutput
)

var ebpfMapOpsErrorsGauge = prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfMapTelemetryNS), "Failures of map operations for a specific ebpf map reported per error.", []string{"map_name", "error"}, nil)
var ebpfHelperErrorsGauge = prometheus.NewDesc(fmt.Sprintf("%s__errors", ebpfHelperTelemetryNS), "Failures of bpf helper operations reported per helper per error for each probe.", []string{"helper", "probe_name", "error"}, nil)

var helperNames = map[int]string{
	readIndx:        "bpf_probe_read",
	readUserIndx:    "bpf_probe_read_user",
	readKernelIndx:  "bpf_probe_read_kernel",
	skbLoadBytes:    "bpf_skb_load_bytes",
	perfEventOutput: "bpf_perf_event_output",
}

// EBPFTelemetry struct contains all the maps that
// are registered to have their telemetry collected.
type EBPFTelemetry struct {
	mtx          sync.Mutex
	mapErrMap    *ebpf.Map
	helperErrMap *ebpf.Map
	mapKeys      map[string]uint64
	probeKeys    map[string]uint64
}

// NewEBPFTelemetry initializes a new EBPFTelemetry object
func NewEBPFTelemetry() *EBPFTelemetry {
	if supported, _ := ebpfTelemetrySupported(); !supported {
		return nil
	}
	return &EBPFTelemetry{
		mapKeys:   make(map[string]uint64),
		probeKeys: make(map[string]uint64),
	}
}

// populateMapsWithKeys initializes the maps for holding telemetry info.
// It must be called after the manager is initialized
func (b *EBPFTelemetry) populateMapsWithKeys(m *manager.Manager) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// first manager to call will populate the maps
	if b.mapErrMap == nil {
		b.mapErrMap, _, _ = m.GetMap(probes.MapErrTelemetryMap)
	}
	if b.helperErrMap == nil {
		b.helperErrMap, _, _ = m.GetMap(probes.HelperErrTelemetryMap)
	}

	if err := b.initializeMapErrTelemetryMap(m.Maps); err != nil {
		return err
	}
	if err := b.initializeHelperErrTelemetryMap(); err != nil {
		return err
	}
	return nil
}

// Describe returns all descriptions of the collector
func (b *EBPFTelemetry) Describe(ch chan<- *prometheus.Desc) {
	ch <- ebpfMapOpsErrorsGauge
	ch <- ebpfHelperErrorsGauge
}

// Collect returns the current state of all metrics of the collector
func (b *EBPFTelemetry) Collect(ch chan<- prometheus.Metric) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.helperErrMap != nil {
		var hval HelperErrTelemetry
		for probeName, k := range b.probeKeys {
			err := b.helperErrMap.Lookup(unsafe.Pointer(&k), unsafe.Pointer(&hval))
			if err != nil {
				log.Debugf("failed to get telemetry for probe:key %s:%d\n", probeName, k)
				continue
			}
			for indx, helperName := range helperNames {
				base := maxErrno * indx
				if count := getErrCount(hval.Count[base : base+maxErrno]); len(count) > 0 {
					for errStr, errCount := range count {
						ch <- prometheus.MustNewConstMetric(ebpfHelperErrorsGauge, prometheus.GaugeValue, float64(errCount), helperName, probeName, errStr)
					}
				}
			}
		}
	}

	if b.mapErrMap != nil {
		var val MapErrTelemetry
		for m, k := range b.mapKeys {
			err := b.mapErrMap.Lookup(unsafe.Pointer(&k), unsafe.Pointer(&val))
			if err != nil {
				log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
				continue
			}
			if count := getErrCount(val.Count[:]); len(count) > 0 {
				for errStr, errCount := range count {
					ch <- prometheus.MustNewConstMetric(ebpfMapOpsErrorsGauge, prometheus.GaugeValue, float64(errCount), m, errStr)
				}
			}
		}
	}
}

func getErrCount(v []uint64) map[string]uint64 {
	errCount := make(map[string]uint64)
	for i, count := range v {
		if count == 0 {
			continue
		}

		if (i + 1) == maxErrno {
			errCount[maxErrnoStr] = count
		} else if name := unix.ErrnoName(syscall.Errno(i)); name != "" {
			errCount[name] = count
		} else {
			errCount[syscall.Errno(i).Error()] = count
		}
	}
	return errCount
}

func buildMapErrTelemetryConstants(mgr *manager.Manager) []manager.ConstantEditor {
	var keys []manager.ConstantEditor
	h := keyHash()
	for _, m := range mgr.Maps {
		keys = append(keys, manager.ConstantEditor{
			Name:  m.Name + "_telemetry_key",
			Value: mapKey(h, m),
		})
	}
	return keys
}

func keyHash() hash.Hash64 {
	return fnv.New64a()
}

func mapKey(h hash.Hash64, m *manager.Map) uint64 {
	h.Reset()
	_, _ = h.Write([]byte(m.Name))
	return h.Sum64()
}

func probeKey(h hash.Hash64, funcName string) uint64 {
	h.Reset()
	_, _ = h.Write([]byte(funcName))
	return h.Sum64()
}

func (b *EBPFTelemetry) initializeMapErrTelemetryMap(maps []*manager.Map) error {
	if b.mapErrMap == nil {
		return nil
	}

	z := new(MapErrTelemetry)
	h := keyHash()
	for _, m := range maps {
		// Some maps, such as the telemetry maps, are
		// redefined in multiple programs.
		if _, ok := b.mapKeys[m.Name]; ok {
			continue
		}

		key := mapKey(h, m)
		err := b.mapErrMap.Update(unsafe.Pointer(&key), unsafe.Pointer(z), ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m.Name)
		}
		b.mapKeys[m.Name] = key
	}
	return nil
}

func (b *EBPFTelemetry) initializeHelperErrTelemetryMap() error {
	if b.helperErrMap == nil {
		return nil
	}

	// the `probeKeys` get added during instruction patching, so we just try to insert entries for any that don't exist
	z := new(HelperErrTelemetry)
	for p, key := range b.probeKeys {
		err := b.helperErrMap.Update(unsafe.Pointer(&key), unsafe.Pointer(z), ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to initialize telemetry struct for probe %s", p)
		}
	}
	return nil
}

func patchEBPFTelemetry(m *manager.Manager, enable bool, bpfTelemetry *EBPFTelemetry) error {
	const symbol = "telemetry_program_id_key"
	newIns := asm.Mov.Reg(asm.R1, asm.R1)
	if enable {
		newIns = asm.StoreXAdd(asm.R1, asm.R2, asm.Word)
	}
	ldDWImm := asm.LoadImmOp(asm.DWord)
	h := keyHash()

	progs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}

	for fn, p := range progs {
		// do constant editing of programs for helper errors post-init
		ins := p.Instructions
		if enable && bpfTelemetry != nil {
			offsets := ins.ReferenceOffsets()
			indices := offsets[symbol]
			if len(indices) > 0 {
				for _, index := range indices {
					load := &ins[index]
					if load.OpCode != ldDWImm {
						return fmt.Errorf("symbol %v: load: found %v instead of %v", symbol, load.OpCode, ldDWImm)
					}
					key := probeKey(h, fn)
					load.Constant = int64(key)
					bpfTelemetry.probeKeys[fn] = key
				}
			}
		}

		// patch telemetry helper calls
		const ebpfTelemetryPatchCall = -1
		iter := ins.Iterate()
		for iter.Next() {
			ins := iter.Ins
			if !ins.IsBuiltinCall() || ins.Constant != ebpfTelemetryPatchCall {
				continue
			}
			*ins = newIns.WithMetadata(ins.Metadata)
		}
	}
	return nil
}

// setupForTelemetry sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
// This must be called before ebpf-manager.Manager.Init/InitWithOptions
func setupForTelemetry(m *manager.Manager, options *manager.Options, bpfTelemetry *EBPFTelemetry) error {
	activateBPFTelemetry, err := ebpfTelemetrySupported()
	if err != nil {
		return err
	}
	m.InstructionPatcher = func(m *manager.Manager) error {
		return patchEBPFTelemetry(m, activateBPFTelemetry, bpfTelemetry)
	}

	if activateBPFTelemetry {
		// add telemetry maps to list of maps, if not present
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == probes.MapErrTelemetryMap }) {
			m.Maps = append(m.Maps, &manager.Map{Name: probes.MapErrTelemetryMap})
		}
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == probes.HelperErrTelemetryMap }) {
			m.Maps = append(m.Maps, &manager.Map{Name: probes.HelperErrTelemetryMap})
		}

		if bpfTelemetry != nil {
			bpfTelemetry.setupMapEditors(options)
		}

		options.ConstantEditors = append(options.ConstantEditors, buildMapErrTelemetryConstants(m)...)
	}
	// we cannot exclude the telemetry maps because on some kernels, deadcode elimination hasn't removed references
	// if telemetry not enabled: leave key constants as zero, and deadcode elimination should reduce number of instructions

	return nil
}

func (b *EBPFTelemetry) setupMapEditors(opts *manager.Options) {
	if (b.mapErrMap != nil) || (b.helperErrMap != nil) {
		if opts.MapEditors == nil {
			opts.MapEditors = make(map[string]*ebpf.Map)
		}
	}
	// if the maps have already been loaded, setup editors to point to them
	if b.mapErrMap != nil {
		opts.MapEditors[probes.MapErrTelemetryMap] = b.mapErrMap
	}
	if b.helperErrMap != nil {
		opts.MapEditors[probes.HelperErrTelemetryMap] = b.helperErrMap
	}
}

// ebpfTelemetrySupported returns whether eBPF telemetry is supported, which depends on the verifier in 4.14+
func ebpfTelemetrySupported() (bool, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return false, err
	}
	return kversion >= kernel.VersionCode(4, 14, 0), nil
}
