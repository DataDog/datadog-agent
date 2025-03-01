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
	"math"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	readIndx int = iota
	readUserIndx
	readKernelIndx
	skbLoadBytes
	perfEventOutput
	ringbufOutput
	ringbufReserve
	mapErr = math.MaxInt
)

var helperNames = map[int]string{
	readIndx:        "bpf_probe_read",
	readUserIndx:    "bpf_probe_read_user",
	readKernelIndx:  "bpf_probe_read_kernel",
	skbLoadBytes:    "bpf_skb_load_bytes",
	perfEventOutput: "bpf_perf_event_output",
	ringbufOutput:   "bpf_ringbuf_output",
	ringbufReserve:  "bpf_ringbuf_reserve",
}

type telemetryKey struct {
	resourceName names.ResourceName
	moduleName   names.ModuleName
}

func (k *telemetryKey) bytes() []byte {
	return []byte(k.String())
}

func (k *telemetryKey) String() string {
	return fmt.Sprintf("%s,%s", k.resourceName.Name(), k.moduleName.Name())
}

// ebpfErrorsTelemetry interface allows easy mocking for UTs without a need to initialize the whole ebpf sub-system and execute ebpf maps APIs
type ebpfErrorsTelemetry interface {
	sync.Locker
	fill([]names.MapName, names.ModuleName, *maps.GenericMap[uint64, mapErrTelemetry], *maps.GenericMap[uint64, helperErrTelemetry]) error
	cleanup([]names.MapName, names.ModuleName, *maps.GenericMap[uint64, mapErrTelemetry], *maps.GenericMap[uint64, helperErrTelemetry]) error
	setProbe(name telemetryKey, hash uint64)
	isInitialized() bool
	forEachMapErrorEntryInMaps(yield func(telemetryKey, uint64, mapErrTelemetry) bool)
	forEachHelperErrorEntryInMaps(yield func(telemetryKey, uint64, helperErrTelemetry) bool)
}

// ebpfTelemetry struct implements ebpfErrorsTelemetry interface and contains all the maps that
// are registered to have their telemetry collected.
type ebpfTelemetry struct {
	mtx                   sync.Mutex
	mapKeys               map[telemetryKey]uint64
	probeKeys             map[telemetryKey]uint64
	mapErrMapsByModule    map[names.ModuleName]*maps.GenericMap[uint64, mapErrTelemetry]
	helperErrMapsByModule map[names.ModuleName]*maps.GenericMap[uint64, helperErrTelemetry]
	initialized           bool
}

// Lock is part of the Locker interface implementation.
func (e *ebpfTelemetry) Lock() {
	e.mtx.Lock()
}

// Unlock is part of the Locker interface implementation.
func (e *ebpfTelemetry) Unlock() {
	e.mtx.Unlock()
}

// fill initializes the maps for holding telemetry info.
// It must be called after the manager is initialized
func (e *ebpfTelemetry) fill(maps []names.MapName, mn names.ModuleName, mapErrMap *maps.GenericMap[uint64, mapErrTelemetry], helperErrMap *maps.GenericMap[uint64, helperErrTelemetry]) error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if err := e.initializeMapErrTelemetryMap(maps, mn, mapErrMap); err != nil {
		return err
	}
	if err := e.initializeHelperErrTelemetryMap(mn, helperErrMap); err != nil {
		return err
	}

	if _, ok := e.mapErrMapsByModule[mn]; ok {
		return fmt.Errorf("eBPF map for map-operation errors for module %s already exists", mn.Name())
	}
	e.mapErrMapsByModule[mn] = mapErrMap

	if _, ok := e.helperErrMapsByModule[mn]; ok {
		return fmt.Errorf("eBPF map for helper-operation errors for module %s already exists", mn.Name())
	}
	e.helperErrMapsByModule[mn] = helperErrMap

	e.initialized = true

	return nil
}

func (e *ebpfTelemetry) cleanup(maps []names.MapName, mn names.ModuleName, mapErrMap *maps.GenericMap[uint64, mapErrTelemetry], helperErrMap *maps.GenericMap[uint64, helperErrTelemetry]) error {
	var errs error

	e.mtx.Lock()
	defer e.mtx.Unlock()

	// Cleanup mapKeys (initialized in initializeMapErrTelemetryMap)
	h := keyHash()
	for _, mapName := range maps {
		delete(e.mapKeys, mapTelemetryKey(mapName, mn))
		key := eBPFMapErrorKey(h, mapTelemetryKey(mapName, mn))
		err := mapErrMap.Delete(&key)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to delete telemetry struct for map %s: %w", mapName, err))
		}
	}

	// Cleanup helper keys
	for p, key := range e.probeKeys {
		if p.moduleName != mn {
			continue
		}
		err := helperErrMap.Delete(&key)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to delete telemetry struct for probe %s: %w", p.String(), err))
		}
		delete(e.probeKeys, p)
	}

	delete(e.mapErrMapsByModule, mn)
	delete(e.helperErrMapsByModule, mn)

	return errs
}

func (e *ebpfTelemetry) setProbe(key telemetryKey, hash uint64) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	e.probeKeys[key] = hash
}

func (e *ebpfTelemetry) isInitialized() bool {
	return e.initialized
}

func (e *ebpfTelemetry) forEachMapErrorEntryInMaps(yield func(key telemetryKey, eBPFKey uint64, val mapErrTelemetry) bool) {
	var mval mapErrTelemetry
	for mod, errMap := range e.mapErrMapsByModule {
		for mKey, k := range e.mapKeys {
			if mod != mKey.moduleName {
				continue
			}

			err := errMap.Lookup(&k, &mval)
			if err != nil {
				log.Debugf("failed to get telemetry %s:%d\n", mKey.String(), k)
				continue
			}
			if !yield(mKey, k, mval) {
				return
			}
		}
	}
}

func (e *ebpfTelemetry) forEachHelperErrorEntryInMaps(yield func(key telemetryKey, eBPFKey uint64, val helperErrTelemetry) bool) {
	var hval helperErrTelemetry
	for mod, errMap := range e.helperErrMapsByModule {
		for pKey, k := range e.probeKeys {
			if mod != pKey.moduleName {
				continue
			}
			err := errMap.Lookup(&k, &hval)
			if err != nil {
				log.Debugf("failed to get telemetry %s:%d\n", pKey.String(), k)
				continue
			}
			if !yield(pKey, k, hval) {
				return
			}
		}
	}
}

// newEBPFTelemetry initializes a new ebpfTelemetry object
func newEBPFTelemetry() ebpfErrorsTelemetry {
	errorsTelemetry = &ebpfTelemetry{
		mapKeys:               make(map[telemetryKey]uint64),
		probeKeys:             make(map[telemetryKey]uint64),
		mapErrMapsByModule:    make(map[names.ModuleName]*maps.GenericMap[uint64, mapErrTelemetry]),
		helperErrMapsByModule: make(map[names.ModuleName]*maps.GenericMap[uint64, helperErrTelemetry]),
	}
	return errorsTelemetry
}

func (e *ebpfTelemetry) initializeMapErrTelemetryMap(maps []names.MapName, mn names.ModuleName, mapErrMap *maps.GenericMap[uint64, mapErrTelemetry]) error {
	z := new(mapErrTelemetry)
	h := keyHash()
	for _, mapName := range maps {
		// Some maps, such as the telemetry maps, are
		// redefined in multiple programs.
		if _, ok := e.mapKeys[mapTelemetryKey(mapName, mn)]; ok {
			continue
		}

		key := eBPFMapErrorKey(h, mapTelemetryKey(mapName, mn))
		err := mapErrMap.Update(&key, z, ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to initialize telemetry struct for map %s: %w", mapName, err)
		}
		e.mapKeys[mapTelemetryKey(mapName, mn)] = key
	}

	return nil
}

func (e *ebpfTelemetry) initializeHelperErrTelemetryMap(module names.ModuleName, helperErrMap *maps.GenericMap[uint64, helperErrTelemetry]) error {
	// the `probeKeys` get added during instruction patching, so we just try to insert entries for any that don't exist
	z := new(helperErrTelemetry)
	for p, key := range e.probeKeys {
		if p.moduleName != module {
			continue
		}

		err := helperErrMap.Update(&key, z, ebpf.UpdateNoExist)
		if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("failed to initialize telemetry struct for probe %s: %w", p.String(), err)
		}
	}

	return nil
}

// PatchConstant replaces the value for the provided relocation entry in the bpf bytecode
func PatchConstant(symbol string, p *ebpf.ProgramSpec, eBPFKey uint64) error {
	// do constant editing of programs for helper errors post-init
	ins := p.Instructions
	ldDWImm := asm.LoadImmOp(asm.DWord)
	offsets := ins.ReferenceOffsets()
	indices := offsets[symbol]
	if len(indices) > 0 {
		for _, index := range indices {
			load := &ins[index]
			if load.OpCode != ldDWImm {
				return fmt.Errorf("symbol %v: load: found %v instead of %v", symbol, load.OpCode, ldDWImm)
			}

			load.Constant = int64(eBPFKey)
		}
	}

	return nil
}

// PatchEBPFTelemetry performs bytecode patching to support eBPF helper error telemetry collection
func PatchEBPFTelemetry(programSpecs map[string]*ebpf.ProgramSpec, enable bool, mn names.ModuleName) error {
	if errorsTelemetry == nil {
		return errors.New("errorsTelemetry not initialized")
	}
	return patchEBPFTelemetry(programSpecs, enable, mn, errorsTelemetry)
}

func patchEBPFTelemetry(programSpecs map[string]*ebpf.ProgramSpec, enable bool, mn names.ModuleName, bpfTelemetry ebpfErrorsTelemetry) error {
	const symbol = "telemetry_program_id_key"
	newIns := asm.Mov.Reg(asm.R1, asm.R1)
	if enable {
		newIns = asm.StoreXAdd(asm.R1, asm.R2, asm.Word)
	}
	h := keyHash()

	for _, p := range programSpecs {
		ins := p.Instructions
		// do constant editing of programs for helper errors post-init
		if enable && bpfTelemetry != nil {
			programName := names.NewProgramNameFromProgramSpec(p)
			tk := probeTelemetryKey(programName, mn)
			key := eBPFHelperErrorKey(h, tk)
			if err := PatchConstant(symbol, p, key); err != nil {
				return err
			}
			bpfTelemetry.setProbe(tk, key)
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

func keyHash() hash.Hash64 {
	return fnv.New64a()
}

func mapTelemetryKey(name names.MapName, mn names.ModuleName) telemetryKey {
	return telemetryKey{resourceName: &name, moduleName: mn}
}

func probeTelemetryKey(programName names.ProgramName, mn names.ModuleName) telemetryKey {
	return telemetryKey{resourceName: &programName, moduleName: mn}
}

func eBPFMapErrorKey(h hash.Hash64, name telemetryKey) uint64 {
	h.Reset()
	_, _ = h.Write(name.bytes())
	return h.Sum64()
}

func eBPFHelperErrorKey(h hash.Hash64, name telemetryKey) uint64 {
	h.Reset()
	_, _ = h.Write(name.bytes())
	return h.Sum64()
}

// EBPFTelemetrySupported returns whether eBPF telemetry is supported, which depends on the verifier in 4.14+
func EBPFTelemetrySupported() (bool, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return false, err
	}
	return kversion >= kernel.VersionCode(4, 14, 0), nil
}
