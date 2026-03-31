// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import "encoding/binary"

// Filter executes a BPF filter program on packet data.
// wirelen is the original packet length; the packet slice may be shorter (captured portion).
// Returns the filter's return value (0 = reject, >0 = number of bytes to accept).
// A nil instruction slice accepts all packets.
//
// This is a port of bpf_filter() from libpcap's bpf_filter.c.
func Filter(insns []Instruction, packet []byte, wirelen uint32) uint32 {
	if len(insns) == 0 {
		return ^uint32(0) // no filter = accept all
	}

	var a, x uint32
	var mem [BPF_MEMWORDS]uint32
	buflen := uint32(len(packet))

	for pc := 0; pc < len(insns); pc++ {
		insn := insns[pc]
		switch insn.Code {

		default:
			return 0

		// Return
		case BPF_RET | BPF_K:
			return insn.K
		case BPF_RET | BPF_A:
			return a

		// Load word (32-bit big-endian)
		case BPF_LD | BPF_W | BPF_ABS:
			k := insn.K
			if k > buflen || 4 > buflen-k {
				return 0
			}
			a = binary.BigEndian.Uint32(packet[k:])
		case BPF_LD | BPF_W | BPF_IND:
			k := x + insn.K
			if insn.K > buflen || x > buflen-insn.K || 4 > buflen-k {
				return 0
			}
			a = binary.BigEndian.Uint32(packet[k:])

		// Load half-word (16-bit big-endian)
		case BPF_LD | BPF_H | BPF_ABS:
			k := insn.K
			if k > buflen || 2 > buflen-k {
				return 0
			}
			a = uint32(binary.BigEndian.Uint16(packet[k:]))
		case BPF_LD | BPF_H | BPF_IND:
			k := x + insn.K
			if x > buflen || insn.K > buflen-x || 2 > buflen-k {
				return 0
			}
			a = uint32(binary.BigEndian.Uint16(packet[k:]))

		// Load byte
		case BPF_LD | BPF_B | BPF_ABS:
			k := insn.K
			if k >= buflen {
				return 0
			}
			a = uint32(packet[k])
		case BPF_LD | BPF_B | BPF_IND:
			k := x + insn.K
			if insn.K >= buflen || x >= buflen-insn.K {
				return 0
			}
			a = uint32(packet[k])

		// Load wire length
		case BPF_LD | BPF_W | BPF_LEN:
			a = wirelen
		case BPF_LDX | BPF_W | BPF_LEN:
			x = wirelen

		// Load immediate
		case BPF_LD | BPF_IMM:
			a = insn.K
		case BPF_LDX | BPF_IMM:
			x = insn.K

		// Load from scratch memory
		case BPF_LD | BPF_MEM:
			a = mem[insn.K]
		case BPF_LDX | BPF_MEM:
			x = mem[insn.K]

		// LDX MSH: X = (packet[k] & 0xf) << 2
		case BPF_LDX | BPF_MSH | BPF_B:
			k := insn.K
			if k >= buflen {
				return 0
			}
			x = uint32(packet[k]&0xf) << 2

		// Store to scratch memory
		case BPF_ST:
			mem[insn.K] = a
		case BPF_STX:
			mem[insn.K] = x

		// Unconditional jump
		case BPF_JMP | BPF_JA:
			pc += int(int32(insn.K)) // sign-extend for backward jumps (ip6 protochain)

		// Conditional jumps (K)
		case BPF_JMP | BPF_JGT | BPF_K:
			if a > insn.K {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}
		case BPF_JMP | BPF_JGE | BPF_K:
			if a >= insn.K {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}
		case BPF_JMP | BPF_JEQ | BPF_K:
			if a == insn.K {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}
		case BPF_JMP | BPF_JSET | BPF_K:
			if a&insn.K != 0 {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}

		// Conditional jumps (X)
		case BPF_JMP | BPF_JGT | BPF_X:
			if a > x {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}
		case BPF_JMP | BPF_JGE | BPF_X:
			if a >= x {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}
		case BPF_JMP | BPF_JEQ | BPF_X:
			if a == x {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}
		case BPF_JMP | BPF_JSET | BPF_X:
			if a&x != 0 {
				pc += int(insn.Jt)
			} else {
				pc += int(insn.Jf)
			}

		// ALU operations (X source)
		case BPF_ALU | BPF_ADD | BPF_X:
			a += x
		case BPF_ALU | BPF_SUB | BPF_X:
			a -= x
		case BPF_ALU | BPF_MUL | BPF_X:
			a *= x
		case BPF_ALU | BPF_DIV | BPF_X:
			if x == 0 {
				return 0
			}
			a /= x
		case BPF_ALU | BPF_MOD | BPF_X:
			if x == 0 {
				return 0
			}
			a %= x
		case BPF_ALU | BPF_AND | BPF_X:
			a &= x
		case BPF_ALU | BPF_OR | BPF_X:
			a |= x
		case BPF_ALU | BPF_XOR | BPF_X:
			a ^= x
		case BPF_ALU | BPF_LSH | BPF_X:
			if x < 32 {
				a <<= x
			} else {
				a = 0
			}
		case BPF_ALU | BPF_RSH | BPF_X:
			if x < 32 {
				a >>= x
			} else {
				a = 0
			}

		// ALU operations (K source)
		case BPF_ALU | BPF_ADD | BPF_K:
			a += insn.K
		case BPF_ALU | BPF_SUB | BPF_K:
			a -= insn.K
		case BPF_ALU | BPF_MUL | BPF_K:
			a *= insn.K
		case BPF_ALU | BPF_DIV | BPF_K:
			a /= insn.K
		case BPF_ALU | BPF_MOD | BPF_K:
			a %= insn.K
		case BPF_ALU | BPF_AND | BPF_K:
			a &= insn.K
		case BPF_ALU | BPF_OR | BPF_K:
			a |= insn.K
		case BPF_ALU | BPF_XOR | BPF_K:
			a ^= insn.K
		case BPF_ALU | BPF_LSH | BPF_K:
			a <<= insn.K
		case BPF_ALU | BPF_RSH | BPF_K:
			a >>= insn.K

		// Negate
		case BPF_ALU | BPF_NEG:
			a = 0 - a

		// Register transfer
		case BPF_MISC | BPF_TAX:
			x = a
		case BPF_MISC | BPF_TXA:
			a = x
		}
	}
	return 0
}
