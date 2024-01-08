// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetPatchedPrintkEditor returns a ConstantEditor that patches log_debug calls to always print one newline,
// independently of whether bpf_trace_printk adds its own (from Linux 5.9 onwards) or not.
func GetPatchedPrintkEditor() manager.ConstantEditor {
	// The default is to add a newline: better to have two newlines than none.
	lastCharacter := '\n'
	kernelVersion, err := kernel.HostVersion()

	// if we can detect the kernel version and it's later than 5.9.0, set the last
	// character to '\0', deleting the newline added in the log_debug call
	if err == nil && kernelVersion >= kernel.VersionCode(5, 9, 0) {
		lastCharacter = 0 // '\0'
	}

	return manager.ConstantEditor{
		Name:          "log_debug_last_character",
		Value:         uint64(lastCharacter),
		FailOnMissing: false, // No problem if the constant is not there, that just means the log_debug method wasn't used or it isn't a debug build
	}
}

// PatchPrintkNewline patches log_debug calls to always print one newline, no matter what the kernel does.
//
// For context, in kernel 5.9.0, bpf_trace_printk adds a newline automatically to anything it prints
// This means that when we support both older and newer kernels, bpf_printk is going to have
// an inconsistent behavior. To avoid this, we have a wrapper called log_debug (see bpf_helpers_custom.h)
// that adds a newline to the message before calling bpf_trace_printk. In older kernels
// this ensures that a newline is added. In newer ones it would mean that two newlines are
// added, so this patcher removes that newline in those cases.
func PatchPrintkNewline(m *manager.Manager) error {
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

	// Compute some opcodes we'll need
	callOpCode := asm.Call.Op(asm.ImmSource) // Call to printk always has immediate source
	movImmOpCode := asm.Mov.Op(asm.ImmSource)
	ldDWImmOpCode := asm.LoadImmOp(asm.DWord)
	movRegOpCode := asm.Mov.Op(asm.RegSource)
	addImmOpCode := asm.Add.Op(asm.ImmSource)

	for _, p := range progs {
		for idx, ins := range p.Instructions {
			if !ins.IsBuiltinCall() || ins.Constant != int64(asm.FnTracePrintk) {
				continue // Not a call to bpf_trace_printk, skip
			}
			maxLookback := max(0, idx-100) // For safety, don't look back more than 100 instructions

			// We found the call to bpf_trace_printk, now we need to find
			// the string on the stack and patch it.
			// For that, find first the second register, which is the second argument
			// to the call, which is the length of the string.
			var lengthLoadIns *asm.Instruction
			for i := idx - 1; i >= maxLookback; i-- {
				candidate := &p.Instructions[i]
				if candidate.OpCode == movImmOpCode && candidate.Dst == asm.R2 {
					lengthLoadIns = candidate
					break
				}
			}
			if lengthLoadIns == nil {
				err = log.Warnf("Could not find length load instruction for bpf_trace_printk call %d in %s", idx, p.Name)
				continue // Skip this call instruction
			}

			// Now we have to find in which part the stack is the string being stored
			// For that we need to find the mov instruction that puts the stack pointer
			// into r1 and then the add that modifies the stack offset
			var stackOffsetIns *asm.Instruction
			for i := idx - 1; i >= maxLookback && stackOffsetIns == nil; i-- {
				candidate := &p.Instructions[i]
				if candidate.OpCode == movRegOpCode && candidate.Dst == asm.R1 && candidate.Src == asm.RFP {
					// Ok, so we found the instruction that loads the stack pointer into r1
					// From that, advance until we find the add instruction that modifies the stack offset
					for j := i + 1; j < idx; j++ {
						candidate = &p.Instructions[j]
						if candidate.OpCode == addImmOpCode && candidate.Dst == asm.R1 {
							stackOffsetIns = candidate
							break
						}
					}
				}
			}
			if stackOffsetIns == nil {
				err = log.Warnf("Could not find stack offset instruction for bpf_trace_printk call %d in %s", idx, p.Name)
				continue
			}
			newlineOffset := int16(stackOffsetIns.Constant + lengthLoadIns.Constant - 1) // -1 because the last character is the null character

			// Now find which store instruction is responsible for putting the newline character
			stringStoreInsIdx := -1
			inInstructionOffset := 0
			for i := idx - 1; i >= maxLookback; i-- {
				candidate := &p.Instructions[i]
				if candidate.OpCode.Class() == asm.StXClass && candidate.Src == asm.R1 && candidate.Dst == asm.RFP {
					if candidate.OpCode.Size() == asm.InvalidSize {
						err = log.Warnf("BUG: store instruction %v returned asm.InvalidSize", candidate)
						continue
					}

					minOffset := candidate.Offset
					maxOffset := minOffset + int16(candidate.OpCode.Size().Sizeof())

					if newlineOffset >= minOffset && newlineOffset <= maxOffset {
						// We found the store instruction that loads the newline character, exit the loop
						stringStoreInsIdx = i
						inInstructionOffset = int(newlineOffset - minOffset)
						break
					}
				}
			}
			if stringStoreInsIdx == -1 {
				err = log.Warnf("Could not find store instruction for bpf_trace_printk call %d in %s", idx, p.Name)
				continue
			}

			// Now try to find the load instruction that loads the string into the R1 register
			// var stringLoadIns *asm.Instruction
			for i := stringStoreInsIdx - 1; i >= maxLookback; i-- {
				candidate := &p.Instructions[i]
				if (candidate.OpCode == movImmOpCode || candidate.OpCode == ldDWImmOpCode) && candidate.Dst == asm.R1 {
					// This is the load instruction that's putting the newline character on the stack
					// (the second to last one, that's why we check <= 9 and not <= 8)
					mask := uint64(0xff) << (uint64(inInstructionOffset-1) * 8)
					if candidate.Constant&int64(mask) == int64('\n')<<(uint64(inInstructionOffset-1)*8) {
						candidate.Constant &= int64(^mask) // Set the newline byte to 0

						// We've correctly patched this instruction, reduce the length in one byte and continue looking for more
						lengthLoadIns.Constant--
					}
					break
				}
			}
		}
	}

	if err != nil {
		return fmt.Errorf("at least one error happened patching printk calls: %v", err)
	}
	return nil
}
