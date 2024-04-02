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
	"slices"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"

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

const (
	// when building ebpf programs with instrumentation, we specify the `-pg` compiler flag. This instruments
	// a call -1 in the beginning of the bytecode sequence. This 'call -1' is referred to as the
	// trampoline call or patch point.
	ebpfEntryTrampolinePatchCall = -1

	ebpfPathTelemetryInc   = -2
	ebpfFetchTelemetryBlob = -3
)

var helperNames = map[int]string{
	readIndx:        "bpf_probe_read",
	readUserIndx:    "bpf_probe_read_user",
	readKernelIndx:  "bpf_probe_read_kernel",
	skbLoadBytes:    "bpf_skb_load_bytes",
	perfEventOutput: "bpf_perf_event_output",
}

const instrumentationMap string = "bpf_instrumentation_map"

// InstrumentationBlock holds the instructions to be appended
// to the end of eBPF bytecode
type InstrumentationBlock struct {
	code             []*asm.Instruction
	instructionCount int
}

// EBPFTelemetry struct contains all the maps that
// are registered to have their telemetry collected.
type EBPFTelemetry struct {
	// pointer to the ebpf instrumentation map. This has to be
	// exported because the loader library uses reflection to
	// set the value of this field.
	EBPFInstrumentationMap *ebpf.Map `ebpf:"bpf_instrumentation_map"`
	mtx                    sync.Mutex
	mapKeys                map[string]uint32
	mapIndex               uint32
	probeKeys              map[string]uint32
	probeIndex             uint32
	bpfDir                 string
}

// InstrumentationFunctions represents the code from the instrumentation file
// which comprise the instrumentation block
var InstrumentationFunctions = struct {
	Filename  string
	functions []string
}{
	"ebpf_instrumentation",
	[]string{
		"ebpf_instrumentation__trampoline_handler",
	},
}

// A singleton instance of the ebpf telemetry struct. Used by the collector and the ebpf managers (via ErrorsTelemetryModifier).
var errorsTelemetry *EBPFTelemetry

// initEBPFTelemetry initializes a new EBPFTelemetry object
func initEBPFTelemetry(bpfDir string) *EBPFTelemetry {
	errorsTelemetry = NewEBPFTelemetry(bpfDir)
	return errorsTelemetry
}

// NewEBPFTelemetry returns an initialized EBPFTelemetry object
func NewEBPFTelemetry(bpfDir string) *EBPFTelemetry {
	return &EBPFTelemetry{
		mapKeys:   make(map[string]uint32),
		probeKeys: make(map[string]uint32),
		bpfDir:    bpfDir,
	}
}

// taken from: https://github.com/cilium/ebpf/blob/main/asm/opcode.go#L109
// this returns the total number of bytes to encode the instruction
func countRawBPFIns(ins *asm.Instruction) int {
	if ins.OpCode.IsDWordLoad() {
		return 2
	}
	return 1
}

// initializeProbeKeys assignes an integer key to each probe in the manager.
// This key is the index in the array at which telemetry for this probe is kept
// See pkg/ebpf/c/telemetry_types.h for the definition of the struct holding telemetry
func initializeProbeKeys(programs map[string]*ebpf.ProgramSpec, bpfTelemetry *EBPFTelemetry) {
	bpfTelemetry.mtx.Lock()
	defer bpfTelemetry.mtx.Unlock()

	for fn := range programs {
		// This is done before because we do not want a probe index to be equal to 0
		// 0 value for telemetry_program_id_key is used as a guard against unpatched telemetry.
		bpfTelemetry.probeIndex++
		bpfTelemetry.probeKeys[fn] = bpfTelemetry.probeIndex
	}
}

func patchConstant(ins asm.Instructions, symbol string, constant int64) error {
	indices := ins.ReferenceOffsets()[symbol]

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

func patchEBPFInstrumentation(m *manager.Manager, bpfTelemetry *EBPFTelemetry, bytecode io.ReaderAt, shouldSkip func(string) bool, block *InstrumentationBlock) error {
	programs, err := m.GetProgramSpecs()
	if err != nil {
		return fmt.Errorf("unable to get program specs: %w", err)
	}

	return PatchEBPFInstrumentation(programs, bpfTelemetry, bytecode, shouldSkip, block)
}

type patchSite struct {
	ins      *asm.Instruction
	callsite int
	index    int
}

// PatchEBPFInstrumentation accepts eBPF bytecode and patches in the eBPF instrumentation
func PatchEBPFInstrumentation(programs map[string]*ebpf.ProgramSpec, bpfTelemetry *EBPFTelemetry, bytecode io.ReaderAt, shouldSkip func(string) bool, block *InstrumentationBlock) error {
	initializeProbeKeys(programs, bpfTelemetry)

	functions := make(map[string]struct{}, len(programs))
	for fn := range programs {
		functions[fn] = struct{}{}
	}
	sizes, err := parseStackSizesSections(bytecode, functions)
	if err != nil {
		return fmt.Errorf("failed to parse '.stack_sizes' section in file: %w", err)
	}

	for fn, p := range programs {
		space := sizes.stackHas8BytesFree(fn)
		// return error if there is not enough stack space to cache the pointer to the telemetry array
		// if the program is intended to be skipped then we continue since we still need to patch a nop
		if !space && !shouldSkip(p.Name) {
			return fmt.Errorf("Function %s does not have enough free stack space for instrumentation", fn)
		}

		// max trampoline offset is maximum number of instruction from program entry before the
		// trampoline call.
		// The trampoline call can either be the first instruction or the second instruction,
		// It will be the second instruction if r1 is used subsequently in the program, so the compiler
		// has to set rX=r1 before the trampoline call.
		const maxTrampolineOffset = 2
		iter := p.Instructions.Iterate()
		var insCount int
		patchSites := make(map[int64][]patchSite)
		for iter.Next() {
			ins := iter.Ins
			// raw count is needed here because we need the byte offset to calculate jumps
			insCount += countRawBPFIns(ins)

			// We cannot use the helper `IsBuiltinCall()` for the entry trampoline since the compiler does not correctly generate
			// the instrumentation instruction. The loader expects the source and destination register to be set to R0
			// along with the correct opcode, for it to be recognized as a built-in call. However, the `call -1` does
			// not satisfy this requirement. Therefore, we use a looser check relying on the constant to be `-1` to correctly
			// identify the patch point.
			//
			// For reference IsBuiltinCall() => ins.OpCode.JumpOp() == Call && ins.Src == R0 && ins.Dst == R0
			// We keep the first condition and augment it by checking the constant of the call.
			if ins.OpCode.JumpOp() == asm.Call && ins.Constant == ebpfEntryTrampolinePatchCall && iter.Offset <= maxTrampolineOffset {
				patchSites[ins.Constant] = append(patchSites[ins.Constant], patchSite{ins, int(iter.Offset), iter.Index})

				// do not break. We continue looping to get correct 'insCount'
			}

			if ins.IsBuiltinCall() && (ins.Constant == ebpfPathTelemetryInc || ins.Constant == ebpfFetchTelemetryBlob) {
				patchSites[ins.Constant] = append(patchSites[ins.Constant], patchSite{ins, int(iter.Offset), iter.Index})
			}
		}

		// The patcher expects a trampoline call in all programs associated with this manager.
		// No trampoline call is therefore an error condition. If this program is built without
		// instrumentation we should never have come this far.
		if patchSites[ebpfEntryTrampolinePatchCall] == nil {
			return fmt.Errorf("no compiler instrumented patch site found for program %s", p.Name)
		}

		kernelVersionSupported, err := EBPFTelemetrySupported()
		if err != nil {
			return fmt.Errorf("could not determine if kernel version is supported for instrumentation: %w", err)
		}

		// if ebpf telemetry is not supported we patch a NOP instruction instead of the
		// atomic increment. This is because older verifiers will fail when trying to
		// verify this instruction. The NOP instruction is an absolute jump with a 0 offset
		// This is the stand-in for NOP used internally by the verifier
		// https://elixir.bootlin.com/linux/v6.7/source/kernel/bpf/verifier.c#L18582
		atomicIncIns := asm.Instruction{OpCode: asm.Ja.Op(asm.ImmSource), Constant: 0}
		if kernelVersionSupported {
			atomicIncIns = asm.StoreXAdd(asm.R1, asm.R2, asm.Word)
		}
		for patchType, sites := range patchSites {
			if patchType == ebpfPathTelemetryInc {
				for _, site := range sites {
					p.Instructions[site.index] = atomicIncIns.WithMetadata(site.ins.Metadata)
				}
			}

			if patchType == ebpfFetchTelemetryBlob {
				for _, site := range sites {
					if shouldSkip(p.Name) {
						p.Instructions[site.index] = asm.Xor.Reg(asm.R0, asm.R0)
					} else {
						p.Instructions[site.index] = asm.LoadMem(asm.R0, asm.RFP, int16(512)*-1, asm.DWord)
					}
				}
			}

			if patchType == ebpfEntryTrampolinePatchCall {
				// there can only be a single trampoline patch site
				if len(sites) > 1 {
					return fmt.Errorf("discovered more than 1 (%d) trampoline patch sites in program %s", len(sites), fn)
				}

				trampolinePatchSite := sites[0]

				// if a specific program in a manager should not be instrumented,
				// instrument patch point with a immediate store of 0 to the stack slot used for
				// caching the instrumentation blob
				if shouldSkip(p.Name) {
					*trampolinePatchSite.ins = asm.Instruction{OpCode: asm.Ja.Op(asm.ImmSource), Constant: 0}.WithMetadata(trampolinePatchSite.ins.Metadata)
					continue
				}
				trampolineInstruction := asm.Instruction{
					OpCode: asm.OpCode(asm.JumpClass).SetJumpOp(asm.Ja),
					Offset: int16(insCount - trampolinePatchSite.callsite - 1),
				}.WithMetadata(trampolinePatchSite.ins.Metadata)
				p.Instructions[trampolinePatchSite.index] = trampolineInstruction

				// patch constant 'telemetry_program_id_key'
				// This will be patched with the intended index into the helper errors array for this probe.
				// This is done after patching the trampoline call, so that programs which should be skipped do
				// not get this constant set. This would allow deadcode elimination to remove the telemetry block.
				if err := patchConstant(p.Instructions, "telemetry_program_id_key", int64(bpfTelemetry.probeKeys[p.Name])); err != nil {
					return fmt.Errorf("failed to patch constant 'telemetry_program_id_key' for program %s: %w", p.Name, err)
				}

				// absolute jump back to the telemetry patch point to continue normal execution
				retJumpOffset := trampolinePatchSite.callsite - (insCount + block.instructionCount)
				newIns := asm.Instruction{
					OpCode: asm.OpCode(asm.JumpClass).SetJumpOp(asm.Ja),
					Offset: int16(retJumpOffset),
				}

				for _, ins := range block.code {
					p.Instructions = append(p.Instructions, *ins)
				}
				p.Instructions = append(p.Instructions, newIns)
			}
		}
	}

	return nil
}

// BuildInstrumentationBlock build a block of instructions to be appended to the end of bytecode as instrumentation
func BuildInstrumentationBlock(bpfAsset io.ReaderAt, collectionSpec *ebpf.CollectionSpec) (*InstrumentationBlock, error) {
	var block []*asm.Instruction
	var blockCount int

	functions := make(map[string]struct{}, len(collectionSpec.Programs))
	for fn := range collectionSpec.Programs {
		functions[fn] = struct{}{}
	}

	sizes, err := parseStackSizesSections(bpfAsset, functions)
	if err != nil {
		return nil, fmt.Errorf("cannot get stack sizes for instrumnetation block: %w", err)
	}

	// each program in the instrumentation file is a separate instrumentation
	// all instrumentation runs one after the other before returning execution back.
	for _, program := range InstrumentationFunctions.functions {
		if _, ok := collectionSpec.Programs[program]; !ok {
			return nil, fmt.Errorf("no program %s present in instrumentation file %s", program, InstrumentationFunctions.Filename)
		}

		// the instrumentation code should not access the stack other than to cache the telemetry pointer
		// writing to the stack will not cause the program to fail but may cause errors related to
		// accessing uninitialized stack locations to get suppressed
		if sizes[program] > 8 /*bytes*/ {
			return nil, errors.New("instrumentation block cannot perform any stack reads or writes on more than one stack slot")
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
			blockCount += countRawBPFIns(ins)

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

				block = append(block, &newIns)
				continue
			}

			block = append(block, ins)
		}
	}

	return &InstrumentationBlock{
		code:             block,
		instructionCount: blockCount,
	}, nil
}

// setupForTelemetry sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
// This must be called before ebpf-manager.Manager.Init/InitWithOptions
func setupForTelemetry(m *manager.Manager, options *manager.Options, bpfTelemetry *EBPFTelemetry, bytecode io.ReaderAt, shouldSkip func(string) bool) error {
	bpfTelemetry.mtx.Lock()
	defer bpfTelemetry.mtx.Unlock()

	instrumented, err := ELFBuiltWithInstrumentation(bytecode)
	if err != nil {
		return fmt.Errorf("error determining if instrumentation is enabled: %w", err)
	}

	// if the elf file is not instrumented then early return
	if !instrumented {
		return nil
	}

	bpfAsset, err := netbpf.ReadEBPFInstrumentationModule(bpfTelemetry.bpfDir, InstrumentationFunctions.Filename)
	if err != nil {
		return fmt.Errorf("failed to read %s bytecode file: %w", InstrumentationFunctions.Filename, err)
	}
	defer bpfAsset.Close()

	collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bpfAsset)
	if err != nil {
		return fmt.Errorf("failed to load collection spec from reader: %w", err)
	}

	// TODO: This check will be removed when ebpf telemetry is converted to a singleton
	if bpfTelemetry.EBPFInstrumentationMap == nil {
		// get reference to instrumentation map
		if err := collectionSpec.LoadAndAssign(bpfTelemetry, nil); err != nil {
			return fmt.Errorf("failed to load instrumentation maps: %w", err)
		}
	}

	block, err := BuildInstrumentationBlock(bpfAsset, collectionSpec)
	if err != nil {
		return fmt.Errorf("unabled to build instrumentation block: %w", err)
	}

	supported, err := EBPFTelemetrySupported()
	if err != nil {
		return err
	}

	// this function will return true if an ebpf program
	// should be skipped or if instrumentation is not supported
	skipFunctionWithSupported := func() func(string) bool {
		if shouldSkip == nil {
			return func(_ string) bool { return !supported }
		}

		return func(name string) bool {
			return shouldSkip(name) || !supported
		}
	}()

	m.InstructionPatchers = append(m.InstructionPatchers, func(m *manager.Manager) error {
		return patchEBPFInstrumentation(m, bpfTelemetry, bytecode, skipFunctionWithSupported, block)
	})

	// add telemetry maps to list of maps, if not present
	if !slices.ContainsFunc(m.Maps, func(x *manager.Map) bool { return x.Name == instrumentationMap }) {
		m.Maps = append(m.Maps, &manager.Map{Name: instrumentationMap})
	}

	if options.MapEditors == nil {
		options.MapEditors = make(map[string]*ebpf.Map)
	}
	options.MapEditors[instrumentationMap] = bpfTelemetry.EBPFInstrumentationMap

	if supported {
		var keys []manager.ConstantEditor
		for _, m := range m.Maps {
			// This is done before because we do not want a map index to be equal to 0
			// 0 value for map_index is used as a guard against unpatched telemetry.
			bpfTelemetry.mapIndex++
			bpfTelemetry.mapKeys[m.Name] = bpfTelemetry.mapIndex

			keys = append(keys, manager.ConstantEditor{
				Name:  m.Name + "_telemetry_key",
				Value: uint64(bpfTelemetry.mapKeys[m.Name]),
			})

			options.ConstantEditors = append(options.ConstantEditors, keys...)
		}
	}
	// we cannot exclude the telemetry maps because on some kernels, deadcode elimination hasn't removed references
	// if telemetry not enabled: leave key constants as zero, and deadcode elimination should reduce number of instructions

	return nil
}

// EBPFTelemetrySupported returns whether eBPF telemetry is supported, which depends on the verifier in 4.14+
func EBPFTelemetrySupported() (bool, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return false, err
	}
	return kversion >= kernel.VersionCode(4, 18, 0), nil
}

// ELFBuiltWithInstrumentation inspects an eBPF ELF file and determines if
// it has been built with instrumentation
func ELFBuiltWithInstrumentation(bytecode io.ReaderAt) (bool, error) {
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
