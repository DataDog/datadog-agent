// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"fmt"
	"hash/fnv"
	"syscall"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	maxErrno    = 64
	maxErrnoStr = "other"
)

const (
	readIndx int = iota
	readUserIndx
	readKernelIndx
	skbLoadBytes
	perfEventOutput
)

var ebpfMapOpsErrorsGauge = telemetry.NewGauge("ebpf_map_ops", "errors", []string{"map_name", "error"}, "Failures of map operations for a specific ebpf map reported per error.")
var ebpfHelperErrorsGauge = telemetry.NewGauge("ebpf_helpers", "errors", []string{"helper", "probe_name", "error"}, "Failures of bpf helper operations reported per helper per error for each probe.")

var helperNames = map[int]string{readIndx: "bpf_probe_read", readUserIndx: "bpf_probe_read_user", readKernelIndx: "bpf_probe_read_kernel", skbLoadBytes: "bpf_skb_load_bytes", perfEventOutput: "bpf_perf_event_output"}

// EBPFTelemetry struct contains all the maps that
// are registered to have their telemetry collected.
type EBPFTelemetry struct {
	MapErrMap    *ebpf.Map
	HelperErrMap *ebpf.Map
	mapKeys      map[string]uint64
	probeKeys    map[string]uint64
}

// NewEBPFTelemetry initializes a new EBPFTelemetry object
func NewEBPFTelemetry() *EBPFTelemetry {
	return &EBPFTelemetry{
		mapKeys:   make(map[string]uint64),
		probeKeys: make(map[string]uint64),
	}
}

// RegisterEBPFTelemetry initializes the maps for holding telemetry info
func (b *EBPFTelemetry) RegisterEBPFTelemetry(m *manager.Manager) error {
	if b == nil {
		return nil
	}

	if err := b.initializeMapErrTelemetryMap(m.Maps); err != nil {
		return err
	}
	if err := b.initializeHelperErrTelemetryMap(m.Probes); err != nil {
		return err
	}

	return nil
}

// GetMapsTelemetry returns a map of error telemetry for each ebpf map
func (b *EBPFTelemetry) GetMapsTelemetry() map[string]interface{} {
	if b == nil {
		return nil
	}

	var val MapErrTelemetry
	t := make(map[string]interface{})

	for m, k := range b.mapKeys {
		err := b.MapErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
			continue
		}
		if count := getMapErrCount(&val); len(count) > 0 {
			t[m] = count
			for errStr, errCount := range count {
				ebpfMapOpsErrorsGauge.Set(float64(errCount), m, errStr)
			}
		}
	}

	return t
}

// GetHelperTelemetry returns a map of error telemetry for each ebpf program
func (b *EBPFTelemetry) GetHelperTelemetry() map[string]interface{} {
	if b == nil {
		return nil
	}

	var val HelperErrTelemetry
	helperTelemMap := make(map[string]interface{})

	for probeName, k := range b.probeKeys {
		err := b.HelperErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", probeName, k)
			continue
		}

		if t := getHelperTelemetry(&val, probeName, ebpfHelperErrorsGauge); len(t) > 0 {
			helperTelemMap[probeName] = t
		}
	}

	return helperTelemMap
}

func getHelperTelemetry(v *HelperErrTelemetry, probeName string, gauge telemetry.Gauge) map[string]interface{} {
	helper := make(map[string]interface{})

	for indx, helperName := range helperNames {
		if count := getErrCount(v, indx); len(count) > 0 {
			helper[helperName] = count

			for errStr, errCount := range count {
				gauge.Set(float64(errCount), helperName, probeName, errStr)
			}
		}
	}

	return helper
}

func getErrCount(v *HelperErrTelemetry, indx int) map[string]uint64 {
	errCount := make(map[string]uint64)
	for i := 0; i < maxErrno; i++ {
		count := v.Count[(maxErrno*indx)+i]
		if count != 0 {
			if (i + 1) == maxErrno {
				errCount[maxErrnoStr] = count
				continue
			}

			errCount[syscall.Errno(i).Error()] = count
		}
	}

	return errCount
}

func getMapErrCount(v *MapErrTelemetry) map[string]uint64 {
	errCount := make(map[string]uint64)

	for i, count := range v.Count {
		if count == 0 {
			continue
		}

		if (i + 1) == maxErrno {
			errCount[maxErrnoStr] = count
			continue
		}
		errCount[syscall.Errno(i).Error()] = count
	}

	return errCount
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

func (b *EBPFTelemetry) initializeMapErrTelemetryMap(maps []*manager.Map) error {
	z := new(MapErrTelemetry)
	h := fnv.New64a()

	for _, m := range maps {
		// Some maps, such as the telemetry maps, are
		// redefined in multiple programs.
		if _, ok := b.mapKeys[m.Name]; ok {
			continue
		}

		h.Write([]byte(m.Name))
		key := h.Sum64()
		err := b.MapErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m.Name)
		}
		h.Reset()

		b.mapKeys[m.Name] = key

	}

	return nil
}

func (b *EBPFTelemetry) initializeHelperErrTelemetryMap(probes []*manager.Probe) error {
	z := new(HelperErrTelemetry)
	h := fnv.New64a()

	for _, p := range probes {
		// Some hook points, like tcp_sendmsg, are probed in
		// multiple different programs.
		if _, ok := b.probeKeys[p.EBPFFuncName]; ok {
			continue
		}

		h.Write([]byte(p.EBPFFuncName))
		key := h.Sum64()
		err := b.HelperErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", p.EBPFFuncName)
		}
		h.Reset()

		b.probeKeys[p.EBPFFuncName] = key
	}

	return nil
}

const EBPFTelemetryPatchCall = -1

func PatchEBPFTelemetry(m *manager.Manager, enable bool, undefinedProbes []manager.ProbeIdentificationPair) error {
	specs, err := getAllProgramSpecs(m, undefinedProbes)
	if err != nil {
		return err
	}

	if enable {
		patchEBPFTelemetry(m, asm.StoreXAdd(asm.R1, asm.R2, asm.Word), specs)
		return nil
	}

	patchEBPFTelemetry(m, asm.Mov.Reg(asm.R1, asm.R1), specs)
	return nil
}

func getAllProgramSpecs(m *manager.Manager, undefinedProbes []manager.ProbeIdentificationPair) ([]*ebpf.ProgramSpec, error) {
	var specs []*ebpf.ProgramSpec
	for _, p := range m.Probes {
		s, present, err := m.GetProgramSpec(p.ProbeIdentificationPair)
		if err != nil {
			return nil, err
		}
		if present {
			specs = append(specs, s...)
		}

	}

	for _, id := range undefinedProbes {
		s, present, err := m.GetProgramSpec(id)
		if err != nil {
			return nil, err
		}
		if present {
			specs = append(specs, s...)
		}

	}

	return specs, nil
}

func patchEBPFTelemetry(m *manager.Manager, newIns asm.Instruction, specs []*ebpf.ProgramSpec) {
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

			if ins.Constant != EBPFTelemetryPatchCall {
				continue
			}

			*ins = newIns.WithMetadata(ins.Metadata)
		}
	}
}

func ActivateBPFTelemetry(m *manager.Manager, undefinedProbes []manager.ProbeIdentificationPair) error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return err
	}
	activateBPFTelemetry := kv >= kernel.VersionCode(4, 14, 0)
	m.InstructionPatcher = func(m *manager.Manager) error {
		return PatchEBPFTelemetry(m, activateBPFTelemetry, undefinedProbes)
	}

	return nil
}
