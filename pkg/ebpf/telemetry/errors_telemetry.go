// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"golang.org/x/exp/slices"

	netbpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
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

const instrumentationMap string = "bpf_instrumentation_map"

// EBPFTelemetry struct contains all the maps that
// are registered to have their telemetry collected.
type EBPFTelemetry struct {
	EBPFInstrumentationMap *ebpf.Map `ebpf:"bpf_instrumentation_map"`
	mtx                    sync.Mutex
	mapKeys                map[string]uint32
	mapIndex               uint32
	probeKeys              map[string]uint32
	probeIndex             uint32
	bpfDir                 string
}

type eBPFInstrumentation struct {
	filename  string
	functions []string
}

var instrumentation = eBPFInstrumentation{
	"ebpf_instrumentation",
	[]string{
		"ebpf_instrumentation__trampoline_handler",
	},
}

// A singleton instance of the ebpf telemetry struct. Used by the collector and the ebpf managers (via ErrorsTelemetryModifier).
var errorsTelemetry *EBPFTelemetry

// newEBPFTelemetry initializes a new EBPFTelemetry object
func newEBPFTelemetry(bpfDir string) *EBPFTelemetry {
	errorsTelemetry := &EBPFTelemetry{
		mapKeys:   make(map[string]uint32),
		probeKeys: make(map[string]uint32),
		bpfDir:    bpfDir,
	}

	return errorsTelemetry
}

// taken from: https://github.com/cilium/ebpf/blob/main/asm/opcode.go#L109
// this returns the total number of bytes to encode the instruction
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
		bpfTelemetry.probeKeys[fn] = bpfTelemetry.probeIndex
		bpfTelemetry.probeIndex++
	}

	return nil
}

func patchConstant(ins asm.Instructions, symbol string, constant int64) error {
	indices := ins.ReferenceOffsets()[symbol]

	// patch telemetry program id key if required for this instrumentation
	ldDWImm := asm.LoadImmOp(asm.DWord)
	for _, index := range indices {
		load := &ins[index]
		if load.OpCode != ldDWImm {
			return fmt.Errorf("symbol %v: load: found %v instead of %v", symbol, load.OpCode, ldDWImm)
		}
		load.Constant = constant
	}

	return nil
}

func patchEBPFInstrumentation(m *manager.Manager, bpfTelemetry *EBPFTelemetry, bytecode io.ReaderAt) error {
	if err := initializeProbeKeys(m, bpfTelemetry); err != nil {
		return err
	}

	progs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}

	sizes, err := parseStackSizesSections(bytecode, progs)
	if err != nil {
		return fmt.Errorf("failed to parse '.stack_sizes' section in file: %w", err)
	}

	for fn, p := range progs {
		space := sizes.stackHas8BytesFree(fn)
		if !space {
			return fmt.Errorf("Function %s does not have enough free stack space for instrumentation", fn)
		}

		// patch 'telemetry_program_id_key'
		if err := patchConstant(p.Instructions, "telemetry_program_id_key", int64(bpfTelemetry.probeKeys[p.Name])); err != nil {
			return fmt.Errorf("failed to patch constant 'telemetry_program_id_key' for program %s: %w", p.Name, err)
		}

		const ebpfEntryTrampolinePatchCall = -1
		// max trampoline offset is maximum number of instruction from program entry before the
		// trampoline call.
		// The trampoline call can either be the first instruction or the second instruction,
		// It will be the second instruction if r1 is used subsequently in the program, so the compiler
		// has to set rX=r1 before the trampoline patch point.
		const maxTrampolineOffset = 2
		iter := p.Instructions.Iterate()
		var telemetryPatchSite *asm.Instruction
		var insCount, telemetryPatchIndex int

		for iter.Next() {
			ins := iter.Ins
			// raw count is needed here because we need the byte offset to calculate jumps
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

		bpfAsset, err := netbpf.ReadEBPFInstrumentationModule(bpfTelemetry.bpfDir, instrumentation.filename)
		if err != nil {
			return fmt.Errorf("failed to read %s bytecode file: %w", instrumentation.filename, err)
		}

		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bpfAsset)
		if err != nil {
			return fmt.Errorf("failed to load collection spec from reader: %w", err)
		}

		for _, program := range instrumentation.functions {
			if _, ok := collectionSpec.Programs[program]; !ok {
				return fmt.Errorf("no program %s present in instrumentation file %s.o", program, instrumentation.filename)
			}
			ins := collectionSpec.Programs[program].Instructions

			iter := ins.Iterate()
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

		retJumpOffset := telemetryPatchIndex - (insCount + instrumentationBlockCount)

		// absolute jump back to the telemetry patch point to continue normal execution
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
func setupForTelemetry(m *manager.Manager, options *manager.Options, bpfTelemetry *EBPFTelemetry, bytecode io.ReaderAt) error {
	bpfTelemetry.mtx.Lock()
	defer bpfTelemetry.mtx.Unlock()

	//	supported, err := ebpfTelemetrySupported()
	//	if err != nil {
	//		return err
	//	}
	instrumented, err := elfBuiltWithInstrumentation(bytecode)
	if err != nil {
		return fmt.Errorf("error determing if instrumentation is enabled: %w", err)
	}

	// if the elf file is not instrumented then early return
	if !instrumented {
		return nil
	}

	// TODO: This check will be removed when ebpf telemetry is converted to a singleton
	if bpfTelemetry.EBPFInstrumentationMap == nil {
		// get reference to instrumentation map
		bpfAsset, err := netbpf.ReadEBPFInstrumentationModule(bpfTelemetry.bpfDir, instrumentation.filename)
		if err != nil {
			return fmt.Errorf("failed to read %s bytecode file: %w", instrumentation.filename, err)
		}

		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bpfAsset)
		if err != nil {
			return fmt.Errorf("failed to load collection spec from reader: %w", err)
		}

		if err := collectionSpec.LoadAndAssign(bpfTelemetry, nil); err != nil {
			return fmt.Errorf("failed to load instrumentation maps: %w", err)
		}

		if err := initializeInstrumentationMap(bpfTelemetry); err != nil {
			return fmt.Errorf("failed to initialize ebpf instrumentation map: %w", err)
		}
	}

	m.InstructionPatchers = append(m.InstructionPatchers, func(m *manager.Manager) error {
		return patchEBPFInstrumentation(m, bpfTelemetry, bytecode)
	})

	// add telemetry maps to list of maps, if not present
	if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == instrumentationMap }) {
		m.Maps = append(m.Maps, &manager.Map{Name: instrumentationMap})
	}

	if options.MapEditors == nil {
		options.MapEditors = make(map[string]*ebpf.Map)
	}
	options.MapEditors[instrumentationMap] = bpfTelemetry.EBPFInstrumentationMap

	var keys []manager.ConstantEditor
	for _, m := range m.Maps {
		bpfTelemetry.mapKeys[m.Name] = bpfTelemetry.mapIndex
		bpfTelemetry.mapIndex++

		keys = append(keys, manager.ConstantEditor{
			Name:  m.Name + "_telemetry_key",
			Value: uint64(bpfTelemetry.mapKeys[m.Name]),
		})

		options.ConstantEditors = append(options.ConstantEditors, keys...)
	}
	// we cannot exclude the telemetry maps because on some kernels, deadcode elimination hasn't removed references
	// if telemetry not enabled: leave key constants as zero, and deadcode elimination should reduce number of instructions

	return nil
}

func initializeInstrumentationMap(b *EBPFTelemetry) error {
	key := 0
	z := new(InstrumentationBlob)
	err := b.EBPFInstrumentationMap.Update(unsafe.Pointer(&key), z, ebpf.UpdateNoExist)
	if err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
		return fmt.Errorf("failed to initialize telemetry struct: %w", err)
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

func elfBuiltWithInstrumentation(bytecode io.ReaderAt) (bool, error) {
	objFile, err := elf.NewFile(bytecode)
	if err != nil {
		return false, fmt.Errorf("failed to open elf file: %w", err)
	}
	defer objFile.Close()

	const instrumentationSectionName = ".build.instrumentation"
	sec := objFile.Section(instrumentationSectionName)
	// if the section is not present then it was not added during compilation.
	// This means that programs in this ELF are not instrumented.
	if sec == nil {
		return false, nil
	}

	data, err := sec.Data()
	if err != nil {
		return false, fmt.Errorf("failed to get data for section %q: %w", instrumentationSectionName, err)
	}

	if i := bytes.IndexByte(data, 0); i != -1 {
		data = data[:i]
	}
	if string(data[:]) != "enabled" {
		return false, nil
	}

	return true, nil
}
