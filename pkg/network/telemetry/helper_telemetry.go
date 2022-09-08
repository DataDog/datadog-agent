// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package telemetry

import (
	"errors"
	"fmt"
	"hash/fnv"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

const (
	maxErrno    = 64
	maxErrnoStr = "other"
)

const (
	readIndx int = iota
	readUserIndx
	readKernelIndx
)

var errIgnore = errors.New("ignore telemetry")

var helperNames = map[int]string{readIndx: "bpf_probe_read", readUserIndx: "bpf_probe_read_user", readKernelIndx: "bpf_probe_read_kernel"}

// BPFTelemetry struct contains all the maps that
// are registered to have their telemetry collected.
type BPFTelemetry struct {
	MapErrMap      *ebpf.Map
	HelperErrMap   *ebpf.Map
	maps           []string
	mapKeys        map[string]uint64
	probes         []string
	probeKeys      map[string]uint64
	shouldActivate bool
}

// NewBPFTelemetry initializes a new BPFTelemetry object
func NewBPFTelemetry(shouldActivate bool) *BPFTelemetry {
	b := new(BPFTelemetry)
	b.mapKeys = make(map[string]uint64)
	b.probeKeys = make(map[string]uint64)
	b.shouldActivate = shouldActivate

	return b
}

// RegisterMaps registers a ebpf map entry in the map error telemetry map,
// to have failing operation telemetry recorded.
func (b *BPFTelemetry) RegisterMaps(maps []string) error {
	if !b.shouldActivate {
		return nil
	}
	b.maps = append(b.maps, maps...)
	return b.initializeMapErrTelemetryMap()
}

// RegisterProbes registers a ebpf map entry in the helper error telemetry map,
// to have failing helper operation telemetry recorded.
func (b *BPFTelemetry) RegisterProbes(probes []string) error {
	if !b.shouldActivate {
		return nil
	}
	b.probes = append(b.probes, probes...)
	return b.initializeHelperErrTelemetryMap()
}

// GetMapsTelemetry returns a map of error telemetry for each ebpf map
func (b *BPFTelemetry) GetMapsTelemetry() map[string]interface{} {
	var val MapErrTelemetry
	t := make(map[string]interface{})

	for m, k := range b.mapKeys {
		err := b.MapErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
		}
		t[m], err = getMapErrCount(&val)
		if err != nil {
			delete(t, m)
		}
	}

	return t
}

// GetHelperTelemetry returns a map of error telemetry for each ebpf program
func (b *BPFTelemetry) GetHelperTelemetry() map[string]interface{} {
	var val HelperErrTelemetry
	t := make(map[string]interface{})

	for m, k := range b.probeKeys {
		err := b.HelperErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
		}
		t[m], err = getHelperTelemetry(&val)
		if err != nil {
			delete(t, m)
		}
	}

	return t
}

func getHelperTelemetry(v *HelperErrTelemetry) (map[string]interface{}, error) {
	var err error
	ignore := errIgnore
	helper := make(map[string]interface{})

	for indx, name := range helperNames {
		helper[name], err = getErrCount(v, indx)
		if err != nil {
			delete(helper, name)
		} else {
			ignore = nil
		}
	}

	return helper, ignore
}

func getErrCount(v *HelperErrTelemetry, indx int) (map[string]uint32, error) {
	errCount := make(map[string]uint32)
	err := errIgnore
	for i := 0; i < maxErrno; i++ {
		count := v.Count[(maxErrno*indx)+i]
		if count != 0 {
			if (i + 1) == maxErrno {
				errCount[maxErrnoStr] = count
			} else {
				errCount[syscall.Errno(i).Error()] = count
			}

			err = nil
		}
	}

	return errCount, err
}

func getMapErrCount(v *MapErrTelemetry) (map[string]uint32, error) {
	errCount := make(map[string]uint32)

	err := errIgnore
	for i, count := range v.Count {
		if count == 0 {
			continue
		}

		if (i + 1) == maxErrno {
			errCount[maxErrnoStr] = count
		} else {
			errCount[syscall.Errno(i).Error()] = count
		}
		err = nil
	}

	return errCount, err
}

// BuildTelemetryKeys returns the keys used to index the maps holding telemetry
// information for bpf helper errors.
func BuildTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
	keys := buildMapErrTelemetryKeys(mgr)
	return append(buildHelperErrTelemetryKeys(mgr), keys...)
}

func buildMapErrTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
	var keys []manager.ConstantEditor

	h := fnv.New64a()
	for _, m := range mgr.Maps {
		h.Write([]byte(m.Name))
		keys = append(keys, manager.ConstantEditor{
			Name:  m.Name + "_telemetry_key",
			Value: h.Sum64(),
		})
		h.Reset()
	}

	return keys
}

func buildHelperErrTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
	var keys []manager.ConstantEditor

	h := fnv.New64a()
	for _, p := range mgr.Probes {
		h.Write([]byte(p.EBPFFuncName))
		keys = append(keys, manager.ConstantEditor{
			Name:  "telemetry_program_id_key",
			Value: h.Sum64(),
		})
		h.Reset()
	}

	return keys
}

func (b *BPFTelemetry) initializeMapErrTelemetryMap() error {
	z := new(MapErrTelemetry)
	h := fnv.New64a()

	for _, m := range b.maps {
		h.Write([]byte(m))
		key := h.Sum64()
		err := b.MapErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m)
		}
		h.Reset()

		b.mapKeys[m] = key
	}

	return nil
}

func (b *BPFTelemetry) initializeHelperErrTelemetryMap() error {
	z := new(HelperErrTelemetry)
	h := fnv.New64a()

	for _, p := range b.probes {
		h.Write([]byte(p))
		key := h.Sum64()
		err := b.HelperErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", p)
		}
		h.Reset()

		b.probeKeys[p] = key
	}

	return nil
}

const BPFTelemetryPatchCall = -1

func PatchBPFTelemetry(m *manager.Manager, shouldActivate bool, undefinedProbes []manager.ProbeIdentificationPair) error {
	specs, err := getAllProgramSpecs(m, undefinedProbes)
	if err != nil {
		return err
	}

	if shouldActivate {
		patchBPFTelemetry(m, asm.StoreXAdd(asm.R1, asm.R2, asm.Word), specs)
	} else {
		patchBPFTelemetry(m, asm.Mov.Reg(asm.R1, asm.R1), specs)
	}

	return nil
}

func getAllProgramSpecs(m *manager.Manager, undefinedProbes []manager.ProbeIdentificationPair) ([]*ebpf.ProgramSpec, error) {
	var specs []*ebpf.ProgramSpec
	for _, p := range m.Probes {
		s, present, err := m.GetProgramSpec(p.ProbeIdentificationPair)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, fmt.Errorf("could not find ProgramSpec for probe %v", p.ProbeIdentificationPair)
		}

		specs = append(specs, s...)
	}

	for _, id := range undefinedProbes {
		s, present, err := m.GetProgramSpec(id)
		if err != nil {
			return nil, err
		}
		if !present {
			return nil, fmt.Errorf("could not find ProgramSpec for probe %v", id)
		}

		specs = append(specs, s...)
	}

	return specs, nil
}

func patchBPFTelemetry(m *manager.Manager, newIns asm.Instruction, specs []*ebpf.ProgramSpec) {
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		iter := spec.Instructions.Iterate()
		for iter.Next() {
			ins := iter.Ins

			if !ins.IsBuiltinCall() {
				continue
			}

			if ins.Constant != BPFTelemetryPatchCall {
				continue
			}

			*ins = newIns
		}
	}
}
