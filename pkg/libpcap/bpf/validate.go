// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

// Validate checks that a BPF program is well-formed.
// It verifies that jumps target valid instructions, memory accesses are
// in range, and the program ends with a return instruction.
//
// Port of bpf_validate() from libpcap's bpf_filter.c.
func Validate(insns []Instruction) bool {
	n := len(insns)
	if n < 1 {
		return false
	}

	for i := 0; i < n; i++ {
		p := insns[i]
		switch Class(p.Code) {

		case BPF_LD, BPF_LDX:
			switch Mode(p.Code) {
			case BPF_IMM:
			case BPF_ABS, BPF_IND, BPF_MSH:
				// Runtime bounds check is sufficient
			case BPF_MEM:
				if p.K >= BPF_MEMWORDS {
					return false
				}
			case BPF_LEN:
			default:
				return false
			}

		case BPF_ST, BPF_STX:
			if p.K >= BPF_MEMWORDS {
				return false
			}

		case BPF_ALU:
			switch Op(p.Code) {
			case BPF_ADD, BPF_SUB, BPF_MUL,
				BPF_OR, BPF_AND, BPF_XOR,
				BPF_LSH, BPF_RSH, BPF_NEG:
			case BPF_DIV, BPF_MOD:
				if Src(p.Code) == BPF_K && p.K == 0 {
					return false
				}
			default:
				return false
			}

		case BPF_JMP:
			from := uint32(i + 1)
			switch Op(p.Code) {
			case BPF_JA:
				if from+p.K >= uint32(n) {
					return false
				}
			case BPF_JEQ, BPF_JGT, BPF_JGE, BPF_JSET:
				if from+uint32(p.Jt) >= uint32(n) || from+uint32(p.Jf) >= uint32(n) {
					return false
				}
			default:
				return false
			}

		case BPF_RET:

		case BPF_MISC:

		default:
			return false
		}
	}

	return Class(insns[n-1].Code) == BPF_RET
}
