// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"errors"
	"fmt"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"golang.org/x/exp/slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	netbpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	readIndx int = iota
	readUserIndx
	readKernelIndx
	skbLoadBytes
	perfEventOutput
)

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
	mtx             sync.Mutex
	bpfTelemetryMap *maps.GenericMap[uint32, InstrumentationBlob]
	mapKeys         map[string]uint32
	mapIndex        uint32
	probeKeys       map[string]uint32
	programIndex    uint32
	bpfDir          string
}

type eBPFInstrumentation struct {
	filename  string
	functions []string
}

var instrumentation = []eBPFInstrumentation{
	{
		"ebpf_instrumentation",
		[]string{
			"ebpf_instrumentation__trampoline_handler",
		},
	},
}

// NewEBPFTelemetry initializes a new EBPFTelemetry object
func NewEBPFTelemetry() *EBPFTelemetry {
	if supported, _ := ebpfTelemetrySupported(); !supported {
		return nil
	}
	return &EBPFTelemetry{
		mapKeys:   make(map[string]uint32),
		probeKeys: make(map[string]uint32),
	}
}

// populateMapsWithKeys initializes the maps for holding telemetry info.
// It must be called after the manager is initialized
func (b *EBPFTelemetry) populateMapsWithKeys(m *manager.Manager) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// first manager to call will populate the maps
	if b.bpfTelemetryMap != nil {
		return nil
	}

	var err error
	b.bpfTelemetryMap, err = maps.GetMap[uint32, InstrumentationBlob](m, probes.EBPFTelemetryMap)
	if err != nil {
		return fmt.Errorf("failed to get bpf telemetry map: %w", err)
	}

	var key uint32
	z := new(InstrumentationBlob)
	err = b.bpfTelemetryMap.Update(&key, z, ebpf.UpdateNoExist)
	if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
		return fmt.Errorf("failed to initialize telemetry struct: %w", err)
	}

	return nil
}

func countRawBPFIns(ins *asm.Instruction) int {
	if ins.OpCode.IsDWordLoad() {
		return 2
	}
	return 1
}

func initializeProbeKeys(m *manager.Manager, bpfTelemetry *EBPFTelemetry) error {
	bpfTelemetry.mtx.Lock()
	defer bpfTelemetry.mtx.Unlock()

	progs, err := m.GetProgramSpecs()
	if err != nil {
		return fmt.Errorf("failed to get program specs: %w", err)
	}

	for fn, _ := range progs {
		bpfTelemetry.probeKeys[fn] = bpfTelemetry.programIndex
		bpfTelemetry.programIndex++
	}

	return nil
}

func patchEBPFTelemetry(m *manager.Manager, enable bool, bpfTelemetry *EBPFTelemetry) error {
	if err := initializeProbeKeys(m, bpfTelemetry); err != nil {
		return err
	}

	progs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}

	for _, p := range progs {
		const ebpfEntryTrampolinePatchCall = -1
		// max trampoline offset is maximum number of instruction from program entry before the
		// trampoline call.
		// The trampoline call can either be the first instruction or the second instruction, if
		// r1 is used subsequently in the program. This is because the compiler sets rX= r1 before
		// the trampoline patch point.
		const maxTrampolineOffset = 2
		iter := p.Instructions.Iterate()
		var telemetryPatchSite *asm.Instruction
		var insCount, telemetryPatchIndex int

		for iter.Next() {
			ins := iter.Ins
			insCount += countRawBPFIns(ins)

			// We cannot use the helper `IsBuiltinCall()` for the entry trampoline since the compiler does not correctly generate
			// the instrumentation instruction. The loader expects the source and destination register to be set to R0
			// along with the correct opcode, for it to be recognized as a built-in call. However, the `call -1` does
			// not satisfy this requirement. Therefore, we use a looser check relying on the constant to be `-1` to correctly
			// identify the patch point.
			if ins.OpCode.JumpOp() == asm.Call && ins.Constant == ebpfEntryTrampolinePatchCall && iter.Offset <= maxTrampolineOffset {
				telemetryPatchSite = iter.Ins
				telemetryPatchIndex = int(iter.Offset)
			}
		}

		if telemetryPatchSite == nil {
			return fmt.Errorf("No compiler instrumented patch site found for program %s\n", p.Name)
		}

		trampoline := asm.Instruction{
			OpCode: asm.OpCode(asm.JumpClass).SetJumpOp(asm.Ja),
			Offset: int16(insCount - telemetryPatchIndex - 1),
		}.WithMetadata(telemetryPatchSite.Metadata)
		*telemetryPatchSite = trampoline

		var instrumentationBlock []*asm.Instruction
		var instrumentationBlockCount int
		for _, eBPFInst := range instrumentation {
			bpfAsset, err := netbpf.ReadEBPFTelemetryModule(bpfTelemetry.bpfDir, eBPFInst.filename)
			if err != nil {
				return fmt.Errorf("failed to read %s bytecode file: %w", eBPFInst.filename, err)
			}

			collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bpfAsset)
			if err != nil {
				return fmt.Errorf("failed to load collection spec from reader: %w", err)
			}

			for _, program := range eBPFInst.functions {
				if _, ok := collectionSpec.Programs[program]; !ok {
					return fmt.Errorf("no program %s present in instrumentation file %s.o", program, eBPFInst.filename)
				}
				iter := collectionSpec.Programs[program].Instructions.Iterate()
				for iter.Next() {
					ins := iter.Ins
					// The final instruction in the instrumentation block is `exit`, which we
					// do not want.
					if ins.OpCode.JumpOp() == asm.Exit {
						break
					}
					instrumentationBlockCount += countRawBPFIns(ins)

					// The first instruction has associated func_info btf information. Since
					// the instrumentation is not a function, the verifier will complain that the number of
					// `func_info` objects in the BTF do not match the number of loaded programs:
					// https://elixir.bootlin.com/linux/latest/source/kernel/bpf/verifier.c#L15035
					// To workaround this we create a new instruction object and give it empty metadata.
					if iter.Index == 0 {
						newIns := asm.Instruction{
							OpCode:   ins.OpCode,
							Dst:      ins.Dst,
							Src:      ins.Src,
							Offset:   ins.Offset,
							Constant: ins.Constant,
						}.WithMetadata(asm.Metadata{})

						instrumentationBlock = append(instrumentationBlock, &newIns)
						continue
					}

					instrumentationBlock = append(instrumentationBlock, ins)
				}
			}
		}

		retJumpOffset := telemetryPatchIndex - (insCount + instrumentationBlockCount)

		// absolute jump back to the telemetry patch point
		newIns := asm.Instruction{
			OpCode: asm.OpCode(asm.JumpClass).SetJumpOp(asm.Ja),
			Offset: int16(retJumpOffset),
		}
		instrumentationBlock = append(instrumentationBlock, &newIns)

		for _, ins := range instrumentationBlock {
			p.Instructions = append(p.Instructions, *ins)
		}
	}
	return nil
}

// setupForTelemetry sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
// This must be called before ebpf-manager.Manager.Init/InitWithOptions
func setupForTelemetry(m *manager.Manager, options *manager.Options, bpfTelemetry *EBPFTelemetry) error {
	bpfTelemetry.mtx.Lock()
	defer bpfTelemetry.mtx.Unlock()

	activateBPFTelemetry, err := ebpfTelemetrySupported()
	if err != nil {
		return err
	}

	m.InstructionPatcher = func(m *manager.Manager) error {
		return patchEBPFTelemetry(m, activateBPFTelemetry, bpfTelemetry)
	}

	if activateBPFTelemetry {
		// add telemetry maps to list of maps, if not present
		if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == probes.EBPFTelemetryMap }) {
			m.Maps = append(m.Maps, &manager.Map{Name: probes.EBPFTelemetryMap})
		}

		if bpfTelemetry != nil {
			bpfTelemetry.setupMapEditors(options)
			for _, m := range m.Maps {
				bpfTelemetry.mapKeys[m.Name] = bpfTelemetry.mapIndex
				bpfTelemetry.mapIndex++
			}
		}
	}
	// we cannot exclude the telemetry maps because on some kernels, deadcode elimination hasn't removed references
	// if telemetry not enabled: leave key constants as zero, and deadcode elimination should reduce number of instructions

	return nil
}

func (b *EBPFTelemetry) setupMapEditors(opts *manager.Options) {
	if b.bpfTelemetryMap != nil && opts.MapEditors == nil {
		opts.MapEditors = make(map[string]*ebpf.Map)
		opts.MapEditors[probes.EBPFTelemetryMap] = b.bpfTelemetryMap.Map()
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
