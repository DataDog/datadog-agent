// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bpf implements BPF instruction types, interpreter, and utilities.
// It is a pure Go port of libpcap's bpf.h, bpf_filter.c, bpf_image.c, and bpf_dump.c.
package bpf

// Instruction is a single BPF instruction, matching struct bpf_insn.
type Instruction struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// Program is a compiled BPF program, matching struct bpf_program.
type Program struct {
	Instructions []Instruction
}

// Stmt creates a BPF statement (no jumps).
func Stmt(code uint16, k uint32) Instruction {
	return Instruction{Code: code, K: k}
}

// Jump creates a BPF jump instruction.
func Jump(code uint16, k uint32, jt, jf uint8) Instruction {
	return Instruction{Code: code, Jt: jt, Jf: jf, K: k}
}

// Instruction classes
const (
	BPF_LD   uint16 = 0x00
	BPF_LDX  uint16 = 0x01
	BPF_ST   uint16 = 0x02
	BPF_STX  uint16 = 0x03
	BPF_ALU  uint16 = 0x04
	BPF_JMP  uint16 = 0x05
	BPF_RET  uint16 = 0x06
	BPF_MISC uint16 = 0x07
)

// ld/ldx sizes
const (
	BPF_W uint16 = 0x00 // 32-bit word
	BPF_H uint16 = 0x08 // 16-bit half-word
	BPF_B uint16 = 0x10 // 8-bit byte
)

// ld/ldx modes
const (
	BPF_IMM uint16 = 0x00
	BPF_ABS uint16 = 0x20
	BPF_IND uint16 = 0x40
	BPF_MEM uint16 = 0x60
	BPF_LEN uint16 = 0x80
	BPF_MSH uint16 = 0xa0
)

// alu/jmp operations
const (
	BPF_ADD uint16 = 0x00
	BPF_SUB uint16 = 0x10
	BPF_MUL uint16 = 0x20
	BPF_DIV uint16 = 0x30
	BPF_OR  uint16 = 0x40
	BPF_AND uint16 = 0x50
	BPF_LSH uint16 = 0x60
	BPF_RSH uint16 = 0x70
	BPF_NEG uint16 = 0x80
	BPF_MOD uint16 = 0x90
	BPF_XOR uint16 = 0xa0
)

// jmp conditions
const (
	BPF_JA   uint16 = 0x00
	BPF_JEQ  uint16 = 0x10
	BPF_JGT  uint16 = 0x20
	BPF_JGE  uint16 = 0x30
	BPF_JSET uint16 = 0x40
)

// source operand
const (
	BPF_K uint16 = 0x00
	BPF_X uint16 = 0x08
)

// ret source
const (
	BPF_A uint16 = 0x10
)

// misc operations
const (
	BPF_TAX uint16 = 0x00
	BPF_TXA uint16 = 0x80
)

// BPF_MEMWORDS is the number of scratch memory words.
const BPF_MEMWORDS = 16

// Class extracts the instruction class from an opcode.
func Class(code uint16) uint16 { return code & 0x07 }

// Size extracts the load/store size from an opcode.
func Size(code uint16) uint16 { return code & 0x18 }

// Mode extracts the addressing mode from an opcode.
func Mode(code uint16) uint16 { return code & 0xe0 }

// Op extracts the ALU/JMP operation from an opcode.
func Op(code uint16) uint16 { return code & 0xf0 }

// Src extracts the source operand type from an opcode.
func Src(code uint16) uint16 { return code & 0x08 }

// MiscOp extracts the misc operation from an opcode.
func MiscOp(code uint16) uint16 { return code & 0xf8 }

// Rval extracts the return value source from an opcode.
func Rval(code uint16) uint16 { return code & 0x18 }
