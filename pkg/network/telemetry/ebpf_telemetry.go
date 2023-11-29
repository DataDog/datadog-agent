// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
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
	MapErrMap    *ebpf.Map
	HelperErrMap *ebpf.Map
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

// Describe returns all descriptions of the collector
func (b *EBPFTelemetry) Describe(ch chan<- *prometheus.Desc) {
	ch <- ebpfMapOpsErrorsGauge
	ch <- ebpfHelperErrorsGauge
}

// Collect returns the current state of all metrics of the collector
func (b *EBPFTelemetry) Collect(ch chan<- prometheus.Metric) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	var hval HelperErrTelemetry
	for probeName, k := range b.probeKeys {
		err := b.HelperErrMap.Lookup(unsafe.Pointer(&k), unsafe.Pointer(&hval))
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

	var val MapErrTelemetry
	for m, k := range b.mapKeys {
		err := b.MapErrMap.Lookup(unsafe.Pointer(&k), unsafe.Pointer(&val))
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

// BuildTelemetryKeys returns the keys used to index the maps holding telemetry
// information for bpf helper errors.
func BuildTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
	keys := buildMapErrTelemetryKeys(mgr)
	return append(buildHelperErrTelemetryKeys(mgr), keys...)
}

func buildMapErrTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
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

func buildHelperErrTelemetryKeys(mgr *manager.Manager) []manager.ConstantEditor {
	var keys []manager.ConstantEditor
	h := keyHash()
	for _, p := range mgr.Probes {
		keys = append(keys, manager.ConstantEditor{
			Name:  "telemetry_program_id_key",
			Value: probeKey(h, p),
			ProbeIdentificationPairs: []manager.ProbeIdentificationPair{
				p.ProbeIdentificationPair,
			},
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

func probeKey(h hash.Hash64, m *manager.Probe) uint64 {
	h.Reset()
	_, _ = h.Write([]byte(m.EBPFFuncName))
	return h.Sum64()
}

func (b *EBPFTelemetry) initializeMapErrTelemetryMap(maps []*manager.Map) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	z := new(MapErrTelemetry)
	h := keyHash()
	for _, m := range maps {
		// Some maps, such as the telemetry maps, are
		// redefined in multiple programs.
		if _, ok := b.mapKeys[m.Name]; ok {
			continue
		}

		key := mapKey(h, m)
		err := b.MapErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for map %s", m.Name)
		}
		b.mapKeys[m.Name] = key
	}
	return nil
}

func (b *EBPFTelemetry) initializeHelperErrTelemetryMap(probes []*manager.Probe) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	z := new(HelperErrTelemetry)
	h := keyHash()
	for _, p := range probes {
		// Some hook points, like tcp_sendmsg, are probed in
		// multiple different programs.
		if _, ok := b.probeKeys[p.EBPFFuncName]; ok {
			continue
		}

		key := probeKey(h, p)
		err := b.HelperErrMap.Put(unsafe.Pointer(&key), unsafe.Pointer(z))
		if err != nil {
			return fmt.Errorf("failed to initialize telemetry struct for probe %s", p.EBPFFuncName)
		}
		b.probeKeys[p.EBPFFuncName] = key
	}
	return nil
}

const ebpfTelemetryPatchCall = -1

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
	return slices.DeleteFunc(specs, func(x *ebpf.ProgramSpec) bool {
		return x == nil
	}), nil
}

func patchEBPFTelemetry(m *manager.Manager, enable bool, undefinedProbes []manager.ProbeIdentificationPair) error {
	specs, err := getAllProgramSpecs(m, undefinedProbes)
	if err != nil {
		return err
	}
	newIns := asm.Mov.Reg(asm.R1, asm.R1)
	if enable {
		newIns = asm.StoreXAdd(asm.R1, asm.R2, asm.Word)
	}

	for _, spec := range specs {
		iter := spec.Instructions.Iterate()
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

// ActivateBPFTelemetry registers the instruction patcher with the provided manager if telemetry is supported.
func ActivateBPFTelemetry(m *manager.Manager, undefinedProbes []manager.ProbeIdentificationPair) error {
	activateBPFTelemetry, err := ebpfTelemetrySupported()
	if err != nil {
		return err
	}
	m.InstructionPatcher = func(m *manager.Manager) error {
		return patchEBPFTelemetry(m, activateBPFTelemetry, undefinedProbes)
	}
	return nil
}

// ebpfTelemetrySupported returns whether eBPF telemetry is supported, which depends on the verifier in 4.14+
func ebpfTelemetrySupported() (bool, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return false, err
	}
	return kversion >= kernel.VersionCode(4, 14, 0), nil
}
