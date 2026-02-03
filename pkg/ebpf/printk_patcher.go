// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxLookbackLimit = 150
const maxRecursionLimit = 2

// patchPrintkNewline patches log_debug calls to always print one newline, no matter what the kernel does.
//
// For context, in kernel 5.9.0, bpf_trace_printk adds a newline automatically to anything it prints
// This means that when we support both older and newer kernels, bpf_printk is going to have
// an inconsistent behavior. To avoid this, we have a wrapper called log_debug (see bpf_helpers_custom.h)
// that adds a newline to the message before calling bpf_trace_printk. In older kernels
// this ensures that a newline is added. In newer ones it would mean that two newlines are
// added, so this patcher removes that newline in those cases.
func patchPrintkNewline(m *manager.Manager) error {
	kernelVersion, err := kernel.HostVersion()
	if err != nil {
		return err // can't detect kernel version, don't patch
	}
	if kernelVersion < kernel.VersionCode(5, 9, 0) {
		return nil // Do nothing in older kernels
	}

	progs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}

	var errs []error

	for _, p := range progs {
		patcher := newPrintkPatcher(p)
		_, err := patcher.patch()
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

type printkPatcher struct {
	jumpSources       map[int][]int
	program           *ebpf.ProgramSpec
	indexToRealOffset map[int]int // maps the actual offset (the one used by the verifier and llvm) to the index of that instruction in the slice. Required to account for double-wide instructions when calculating jump offsets
	realOffsetToIndex map[int]int // maps the actual offset (the one used by the verifier and llvm) to the index of that instruction in the slice. Required to account for double-wide instructions when calculating jump offsets

	movImmOpCode  asm.OpCode
	ldDWImmOpCode asm.OpCode
	movRegOpCode  asm.OpCode
	addImmOpCode  asm.OpCode
}

func newPrintkPatcher(p *ebpf.ProgramSpec) *printkPatcher {
	return &printkPatcher{
		jumpSources:       make(map[int][]int),
		program:           p,
		indexToRealOffset: make(map[int]int),
		realOffsetToIndex: make(map[int]int),

		// Precompute some opcodes we'll need
		movImmOpCode:  asm.Mov.Op(asm.ImmSource),
		ldDWImmOpCode: asm.LoadImmOp(asm.DWord),
		movRegOpCode:  asm.Mov.Op(asm.RegSource),
		addImmOpCode:  asm.Add.Op(asm.ImmSource),
	}
}

func (p *printkPatcher) precomputeOffsets() {
	iter := p.program.Instructions.Iterate()
	for iter.Next() {
		idx := int(iter.Offset)
		p.indexToRealOffset[iter.Index] = idx
		p.realOffsetToIndex[idx] = iter.Index
	}
}

func (p *printkPatcher) populateJumpData() {
	for idx, ins := range p.program.Instructions {
		jumpOp := ins.OpCode.JumpOp()
		if jumpOp != asm.Call && jumpOp != asm.Exit && jumpOp != asm.InvalidJumpOp {
			sourceOffset := p.indexToRealOffset[idx]
			targetOffset := sourceOffset + int(ins.Offset) + 1
			targetIdx := p.realOffsetToIndex[targetOffset]

			p.jumpSources[targetIdx] = append(p.jumpSources[targetIdx], idx)
			log.Tracef("Found jump %d -> %d (offsets %d -> %d) in %s: %v", idx, targetIdx, sourceOffset, targetOffset, p.program.Name, ins)
		}
	}

	log.Tracef("Found %d jump sources in %s", len(p.jumpSources), p.program.Name)
}

// patch patches the instructions of a program to remove the newline character
// It's separated from patchPrintkNewline so it can be tested independently, also so that we can
// check how many patches are performed
func (p *printkPatcher) patch() (int, error) {
	var errs []error // list of errors that happened while patching, if any
	numPatches := 0  // number of patches performed

	// Precompute the offsets of the instructions so we can use them to calculate jump offsets properly, accounting for double-wide instructions
	p.precomputeOffsets()

	// Create a map of jump targets to the instruction index that jumps to it so that we
	// can backtrack in case the compiler reuses the same "call 6" instruction for multiple calls to bpf_trace_printk.
	p.populateJumpData()

	// In some cases the compiler might reuse the same instruction for multiple calls to bpf_trace_printk,
	// keep track of that to avoid errors when we don't find a newline in an instruction we've already patched.
	patchedInstructionIndexes := make(map[int]bool)
	patchedLengthLoadIndexes := make(map[int]bool)

	log.Tracef("Start patching instructions for %s", p.program.Name)
	for idx, ins := range p.program.Instructions {
		if !ins.IsBuiltinCall() || ins.Constant != int64(asm.FnTracePrintk) {
			continue // Not a call to bpf_trace_printk, skip
		}
		realOffset := p.indexToRealOffset[idx]
		log.Tracef("Found call to bpf_trace_printk at index %d in %s", realOffset, p.program.Name)

		// For safety, set a limit on look back and recursion depth, to avoid
		// exploring the entire program every time we find a call to
		// bpf_trace_printk.
		requisiteInstructions, err := p.findRequisiteInstructions(idx, maxLookbackLimit, maxRecursionLimit, requisiteInstructions{})
		if len(requisiteInstructions) == 0 && err != nil {
			// Only log errors from the findRequisiteInstructions if we didn't have anything to patch
			errs = append(errs, fmt.Errorf("error finding requisite instructions for bpf_trace_printk call %d in %s: %w", realOffset, p.program.Name, err))
		}

		for _, reqs := range requisiteInstructions {
			// Perform the patching for this set of instructions
			// This is the load instruction that's putting the newline character on the stack
			// Now we need to patch it to put a null character instead
			bitOffset := uint64(reqs.newlineOffsetInInstruction) * 8
			mask := uint64(0xff) << bitOffset
			if reqs.stringLoad.Constant&int64(mask) == int64('\n')<<int64(bitOffset) { // Sanity check: don't overwrite anything if it's not a newline
				reqs.stringLoad.Constant &= int64(^mask) // Set the newline byte to 0

				// We've correctly patched this instruction, reduce the length in one byte if we hadn't done before and continue looking for more
				if !patchedLengthLoadIndexes[reqs.lengthLoadIndex] {
					reqs.lengthLoad.Constant--
					patchedLengthLoadIndexes[reqs.lengthLoadIndex] = true
				}
				patchedInstructionIndexes[reqs.stringLoadIndex] = true // Keep track of which instructions we've patched
				numPatches++
				log.Tracef("Patched instruction %v for bpf_trace_printk call %d in %s", reqs.stringLoad, realOffset, p.program.Name)
			} else if patchedInstructionIndexes[reqs.stringLoadIndex] {
				// We don't have a newline in the expected spot but we've already patched this instruction. The compiler
				// can reuse the same instruction for multiple calls to bpf_trace_printk, and in that case
				// we only need to patch it once. However, we do need to reduce the length in one byte because
				// in some systems the printk call will not be made if the length is not consistent.
				log.Tracef("Instruction %v was already patched for bpf_trace_printk call %d in %s", reqs.stringLoad, realOffset, p.program.Name)
				if !patchedLengthLoadIndexes[reqs.lengthLoadIndex] {
					reqs.lengthLoad.Constant--
					patchedLengthLoadIndexes[reqs.lengthLoadIndex] = true
				}
				numPatches++
			} else {
				errs = append(errs, log.Warnf("Instruction %v (index %d) does not have a newline we can patch for bpf_trace_printk_call %d in %s", reqs.stringLoad, reqs.stringLoadIndex, realOffset, p.program.Name))
			}

		}
	}
	log.Debugf("Patched %d instructions for %s, errors: %v", numPatches, p.program.Name, errs)

	return numPatches, errors.Join(errs...)
}

// requisiteInstructions is a struct that contains the instructions that are necessary for a printk call
// All of these are needed to know to be able to patch the call
type requisiteInstructions struct {
	lengthLoad                 *asm.Instruction // The instruction that loads the length of the string into r2
	lengthLoadIndex            int              // The index of the length load instruction in the program
	stackOffset                *asm.Instruction // The instruction that loads the stack pointer into r1
	stringStore                *asm.Instruction // The instruction that stores the string into the stack from a register
	stringLoad                 *asm.Instruction // The instruction that loads the string from an immediate value to the register
	stringLoadIndex            int              // The index of the string load instruction in the program
	newlineOffsetInInstruction int              // The offset of the newline character in the instruction
}

func (r *requisiteInstructions) isComplete() bool {
	return r.lengthLoad != nil && r.stackOffset != nil && r.stringStore != nil && r.stringLoad != nil
}

func (r *requisiteInstructions) newlineOffset() int16 {
	return int16(r.stackOffset.Constant + r.lengthLoad.Constant - 2) // -1 because the last character is the null character
}

// findRequisiteInstructions finds the instructions that are necessary for a printk call. It will return a list of instruction sets,
// each of which contains the entire sequence of instructions that we need to patch for a bpf_trace_printk call.
func (p *printkPatcher) findRequisiteInstructions(startIdx int, lookbackLimit int, recursionLimit int, currentInstructions requisiteInstructions) ([]requisiteInstructions, error) {
	var errs []error // list of errors that happened while patching, if any
	var foundInstructions []requisiteInstructions

	insLimit := max(startIdx-lookbackLimit, 0)

	// Traverse the instructions backwards from the start. We'll search for the
	// instructions needed by looking at the instruction type. In some cases,
	// we'll only look at a given type of instruction once we have the
	// prerequisites. For example, we need to find the instruction that gives us
	// the offset in the stack where the string is stored before we can look for
	// the instruction that stores the newline character in that stack.
	for idx := startIdx; idx >= insLimit && !currentInstructions.isComplete(); idx-- {
		ins := &p.program.Instructions[idx]
		realOffset := p.indexToRealOffset[idx]

		// If current instruction is an unconditional jump, we can't follow it,
		// so stop (unless it's the first instruction we landed on, in which
		// case it just means it's the source of the jump we took previously)
		if ins.OpCode.JumpOp() == asm.Ja && idx != startIdx {
			log.Tracef("Found unconditional jump instruction %v at %d in %s, stopping search", ins, realOffset, p.program.Name)
			break
		}

		// Inspect the current instruction and see if it's any of the ones we need

		// For the length load instruction, find first the value of the second register, which is the second argument
		// to the call, which is the length of the string.
		// Example instruction: MovImm dst: r2 imm: 0x0000000000000009 which
		// sets the length of the formatting to 9
		if currentInstructions.lengthLoad == nil && ins.OpCode == p.movImmOpCode && ins.Dst == asm.R2 {
			currentInstructions.lengthLoad = ins
			currentInstructions.lengthLoadIndex = idx
			log.Tracef("Found length load instruction %v at %d in %s", ins, realOffset, p.program.Name)
		}

		// For the stack offset instruction that tells us where the string is stored on the stack,
		// we need to find the mov instruction that puts the stack pointer
		// into r1 and then the add that modifies the stack offset
		// We are looking for a sequence like this:
		// MovReg dst: r1 src: rfp                 | Sets r1 to the stack pointer
		// AddImm dst: r1 imm: 0x-000000000000048  | Adjusts the offset
		if currentInstructions.stackOffset == nil && ins.OpCode == p.movRegOpCode && ins.Dst == asm.R1 && ins.Src == asm.RFP {
			// Ok, so we found the instruction that loads the stack pointer into r1
			// From that, advance until we find the add instruction that modifies the stack offset
			// (the AddImm instruction in the example above)
			for j := idx + 1; j < startIdx; j++ {
				candidate := &p.program.Instructions[j]
				if candidate.OpCode == p.addImmOpCode && candidate.Dst == asm.R1 {
					currentInstructions.stackOffset = candidate
					log.Tracef("Found stack offset instruction %v at %d in %s", candidate, realOffset, p.program.Name)
					break
				} else if (candidate.OpCode.Class().IsALU() || candidate.OpCode.Class().IsLoad()) && candidate.Dst == asm.R1 {
					return nil, log.Warnf("Found instruction %v at %d that modifies r1, aborting stack offset search for bpf_trace_printk", candidate, realOffset)
				}
			}
		}

		// For the string store instruction that tells us where the string is stored on the stack,
		// we need to find which store instruction is responsible for putting the newline character on the stack.
		// We will find all store instructions that copy to RFP and check that it's changing the string
		// at the position we expect the newline to be in. After that, we will check the value of the source register.
		// The instruction we're looking for is something like this:
		// StXMemDW dst: rfp src: r1 off: -72 imm: 0x0000000000000000
		// We also need the stackOffset and lengthLoad instructions to be known at this point
		if currentInstructions.stackOffset != nil && currentInstructions.lengthLoad != nil && currentInstructions.stringStore == nil {
			if ins.OpCode.Class() == asm.StXClass && ins.Dst == asm.RFP {
				if ins.OpCode.Size() == asm.InvalidSize {
					errs = append(errs, log.Warnf("BUG: store instruction %v returned asm.InvalidSize", ins))
					continue
				}

				minOffset := ins.Offset
				maxOffset := minOffset + int16(ins.OpCode.Size().Sizeof())
				newlineOffset := currentInstructions.newlineOffset()

				if newlineOffset >= minOffset && newlineOffset < maxOffset {
					// We found the store instruction that loads the newline character, exit the loop
					currentInstructions.stringStore = ins
					currentInstructions.newlineOffsetInInstruction = int(newlineOffset - minOffset)
					log.Tracef("Found string store instruction %v at %d in %s, newlineOffsetInInstruction=%d", ins, realOffset, p.program.Name, currentInstructions.newlineOffsetInInstruction)
				}
			} else if ins.Dst == asm.RFP {
				// Something is modifying the stack pointer and it's not a store
				// instruction. We found it after the stack offset and length
				// loading instructions were found, but before we could find the
				// string store instruction. We cannot be sure any longer that
				// we're in the same call. Abort this search.
				return nil, log.Warnf("Found instruction %v at %d that modifies the stack pointer, aborting search for bpf_trace_printk", ins, realOffset)
			}
		}

		// For the string load instruction that tells us where the string is loaded from,
		// check the one that loads into the register that was used for the store instruction.
		// Something like this:  LdImmDW dst: r1 imm: 0x000a363534333231
		// Note: in hex, the newline character is 0x0a
		if currentInstructions.stringStore != nil {
			targetReg := currentInstructions.stringStore.Src
			if (ins.OpCode == p.movImmOpCode || ins.OpCode == p.ldDWImmOpCode) && ins.Dst == targetReg {
				// This is the load instruction that's putting the newline character on the stack
				// It's the last instruction we needed, so we can return and end this branch of the search here
				currentInstructions.stringLoad = ins
				currentInstructions.stringLoadIndex = idx
				foundInstructions = append(foundInstructions, currentInstructions)
				log.Tracef("Found string load instruction %v at %d in %s", ins, realOffset, p.program.Name)
				log.Tracef("Instruction set complete, stopping search, len(foundInstructions)=%d", len(foundInstructions))

				break
			}
		}

		// If we haven't found all the instructions we need (that is, the break
		// above hasn't been triggered), check if the current instruction is a
		// jump target. If it is, we want to follow the jump source too, unless
		// we've already reached the recursion limit
		sources, ok := p.jumpSources[idx]
		if recursionLimit > 0 && ok {
			newLookbackLimit := lookbackLimit - (startIdx - idx) // Ensure we account for the instructions we already looked back
			newRecursionLimit := recursionLimit - 1

			for _, source := range sources {
				log.Tracef("Backtracking jump from %d to %d (offsets %d -> %d) in %s, new lookback limit: %d, new recursion limit: %d", source, idx, p.indexToRealOffset[source], p.indexToRealOffset[idx], p.program.Name, newLookbackLimit, newRecursionLimit)
				insns, err := p.findRequisiteInstructions(source, newLookbackLimit, newRecursionLimit, currentInstructions)
				foundInstructions = append(foundInstructions, insns...)
				log.Tracef("Finished backtracking jump from %d to %d (offsets %d -> %d) in %s, found %d instruction sets", source, idx, p.indexToRealOffset[source], p.indexToRealOffset[idx], p.program.Name, len(insns))
				if err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	if len(foundInstructions) > 0 {
		return foundInstructions, nil // We found some instructions, return them, ignore errors from other branches
	}
	return nil, errors.Join(errs...)
}

// PrintkPatcherModifier adds an InstructionPatcher to the manager that removes the newline character from log_debug calls if needed
type PrintkPatcherModifier struct {
}

// ensure PrintkPatcherModifier implements the ModifierBeforeInit interface
var _ ModifierBeforeInit = &PrintkPatcherModifier{}

func (t *PrintkPatcherModifier) String() string {
	return "PrintkPatcherModifier"
}

// BeforeInit adds the patchPrintkNewline function to the manager
func (t *PrintkPatcherModifier) BeforeInit(m *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	m.InstructionPatchers = append(m.InstructionPatchers, patchPrintkNewline)
	return nil
}
