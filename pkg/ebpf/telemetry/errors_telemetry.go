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
	"io"
	"math"
	"slices"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	readIndx int = iota
	readUserIndx
	readKernelIndx
	skbLoadBytes
	perfEventOutput
	mapErr = math.MaxInt
)

var helperNames = map[int]string{
	readIndx:        "bpf_probe_read",
	readUserIndx:    "bpf_probe_read_user",
	readKernelIndx:  "bpf_probe_read_kernel",
	skbLoadBytes:    "bpf_skb_load_bytes",
	perfEventOutput: "bpf_perf_event_output",
}

type ebpfErrorsTelemetry interface {
	sync.Locker
	setup(opts *manager.Options)
	fillMaps(m *manager.Manager) error
	getMapKeys() map[string]uint64
	getProbeKeys() map[string]uint64
	setProbe(name string, hash uint64)
	isInitialized() bool
	getMapsTelemetryEntry(key uint64) (MapErrTelemetry, error)
	getHelpersTelemetryEntry(key uint64) (HelperErrTelemetry, error)
}

// EBPFTelemetry struct contains all the maps that
// are registered to have their telemetry collected.
type EBPFTelemetry struct {
	mtx          sync.Mutex
	mapErrMap    *maps.GenericMap[uint64, MapErrTelemetry]
	helperErrMap *maps.GenericMap[uint64, HelperErrTelemetry]
	mapKeys      map[string]uint64
	probeKeys    map[string]uint64
}

func (e *EBPFTelemetry) Lock() {
	e.mtx.Lock()
}

func (e *EBPFTelemetry) Unlock() {
	e.mtx.Unlock()
}

func (e *EBPFTelemetry) setup(opts *manager.Options) {
	e.setupMapEditors(opts)
}

func (e *EBPFTelemetry) fillMaps(m *manager.Manager) error {
	return e.populateMapsWithKeys(m)
}

func (e *EBPFTelemetry) getMapKeys() map[string]uint64 {
	return e.mapKeys
}

func (e *EBPFTelemetry) getProbeKeys() map[string]uint64 {
	return e.probeKeys
}

func (e *EBPFTelemetry) setProbe(name string, hash uint64) {
	e.probeKeys[name] = hash
}

func (e *EBPFTelemetry) isInitialized() bool {
	return e.mapErrMap != nil && e.helperErrMap != nil
}

func (e *EBPFTelemetry) getMapsTelemetryEntry(key uint64) (MapErrTelemetry, error) {
	var val MapErrTelemetry
	err := e.mapErrMap.Lookup(&key, &val)
	return val, err
}

func (e *EBPFTelemetry) getHelpersTelemetryEntry(key uint64) (HelperErrTelemetry, error) {
	var val HelperErrTelemetry
	err := e.helperErrMap.Lookup(&key, &val)
	return val, err
}

// newEBPFTelemetry initializes a new EBPFTelemetry object
func newEBPFTelemetry() ebpfErrorsTelemetry {
	errorsTelemetry = &EBPFTelemetry{
		mapKeys:   make(map[string]uint64),
		probeKeys: make(map[string]uint64),
	}
	return errorsTelemetry
}

func (e *EBPFTelemetry) setupMapEditors(opts *manager.Options) {
	if (e.mapErrMap != nil) || (e.helperErrMap != nil) {
		if opts.MapEditors == nil {
			opts.MapEditors = make(map[string]*ebpf.Map)
		}
	}
	// if the maps have already been loaded, setup editors to point to them
	if e.mapErrMap != nil {
		opts.MapEditors[probes.MapErrTelemetryMap] = e.mapErrMap.Map()
	}
	if e.helperErrMap != nil {
		opts.MapEditors[probes.HelperErrTelemetryMap] = e.helperErrMap.Map()
	}
}

// populateMapsWithKeys initializes the maps for holding telemetry info.
// It must be called after the manager is initialized
func (e *EBPFTelemetry) populateMapsWithKeys(m *manager.Manager) error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	// first manager to call will populate the maps
	if e.mapErrMap == nil {
		e.mapErrMap, _ = maps.GetMap[uint64, MapErrTelemetry](m, probes.MapErrTelemetryMap)
	}
	if e.helperErrMap == nil {
		e.helperErrMap, _ = maps.GetMap[uint64, HelperErrTelemetry](m, probes.HelperErrTelemetryMap)
	}

	if err := e.initializeMapErrTelemetryMap(m.Maps); err != nil {
		return err
	}
	if err := e.initializeHelperErrTelemetryMap(); err != nil {
		return err
	}
	return nil
}

func (e *EBPFTelemetry) initializeMapErrTelemetryMap(maps []*manager.Map) error {
	if e.mapErrMap == nil {
		return nil
	}

	z := new(MapErrTelemetry)
	h := keyHash()
	for _, m := range maps {
		// Some maps, such as the telemetry maps, are
		// redefined in multiple programs.
		if _, ok := e.mapKeys[m.Name]; ok {
			continue
		}

		key := mapKey(h, m)
		err := e.mapErrMap.Update(&key, z, ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m.Name)
		}
		e.mapKeys[m.Name] = key
	}
	return nil
}

func (e *EBPFTelemetry) initializeHelperErrTelemetryMap() error {
	if e.helperErrMap == nil {
		return nil
	}

	// the `probeKeys` get added during instruction patching, so we just try to insert entries for any that don't exist
	z := new(HelperErrTelemetry)
	for p, key := range e.probeKeys {
		err := e.helperErrMap.Update(&key, z, ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to initialize telemetry struct for probe %s", p)
		}
	}
	return nil
}

// setupForTelemetry sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
// This must be called before ebpf-manager.Manager.Init/InitWithOptions
func setupForTelemetry(m *manager.Manager, options *manager.Options, bpfTelemetry ebpfErrorsTelemetry) error {
	activateBPFTelemetry, err := ebpfTelemetrySupported()
	if err != nil {
		return err
	}
	m.InstructionPatchers = append(m.InstructionPatchers, func(m *manager.Manager) error {
		return patchEBPFTelemetry(m, activateBPFTelemetry, bpfTelemetry)
	})

	if activateBPFTelemetry {
		// add telemetry maps to list of maps, if not present
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == probes.MapErrTelemetryMap }) {
			m.Maps = append(m.Maps, &manager.Map{Name: probes.MapErrTelemetryMap})
		}
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == probes.HelperErrTelemetryMap }) {
			m.Maps = append(m.Maps, &manager.Map{Name: probes.HelperErrTelemetryMap})
		}

		if bpfTelemetry != nil {
			bpfTelemetry.setup(options)
		}

		options.ConstantEditors = append(options.ConstantEditors, buildMapErrTelemetryConstants(m)...)
	}
	// we cannot exclude the telemetry maps because on some kernels, deadcode elimination hasn't removed references
	// if telemetry not enabled: leave key constants as zero, and deadcode elimination should reduce number of instructions

	return nil
}

func patchEBPFTelemetry(m *manager.Manager, enable bool, bpfTelemetry ebpfErrorsTelemetry) error {
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
					bpfTelemetry.setProbe(fn, key)
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

// ebpfTelemetrySupported returns whether eBPF telemetry is supported, which depends on the verifier in 4.14+
func ebpfTelemetrySupported() (bool, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return false, err
	}
	return kversion >= kernel.VersionCode(4, 14, 0), nil
}
