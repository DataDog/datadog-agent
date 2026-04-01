// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import (
	"fmt"
	"strconv"
)

// Image returns a human-readable representation of a BPF instruction.
// n is the instruction index (used to compute absolute jump targets).
//
// Port of bpf_image() from libpcap's bpf_image.c.
func Image(p Instruction, n int) string {
	var op, operand string

	switch p.Code {
	default:
		op = "unimp"
		operand = fmt.Sprintf("0x%x", p.Code)

	case BPF_RET | BPF_K:
		op = "ret"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_RET | BPF_A:
		op = "ret"

	case BPF_LD | BPF_W | BPF_ABS:
		op = "ld"
		operand = fmt.Sprintf("[%d]", p.K)
	case BPF_LD | BPF_H | BPF_ABS:
		op = "ldh"
		operand = fmt.Sprintf("[%d]", p.K)
	case BPF_LD | BPF_B | BPF_ABS:
		op = "ldb"
		operand = fmt.Sprintf("[%d]", p.K)

	case BPF_LD | BPF_W | BPF_LEN:
		op = "ld"
		operand = "#pktlen"

	case BPF_LD | BPF_W | BPF_IND:
		op = "ld"
		operand = fmt.Sprintf("[x + %d]", p.K)
	case BPF_LD | BPF_H | BPF_IND:
		op = "ldh"
		operand = fmt.Sprintf("[x + %d]", p.K)
	case BPF_LD | BPF_B | BPF_IND:
		op = "ldb"
		operand = fmt.Sprintf("[x + %d]", p.K)

	case BPF_LD | BPF_IMM:
		op = "ld"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_LDX | BPF_IMM:
		op = "ldx"
		operand = fmt.Sprintf("#0x%x", p.K)

	case BPF_LDX | BPF_MSH | BPF_B:
		op = "ldxb"
		operand = fmt.Sprintf("4*([%d]&0xf)", p.K)

	case BPF_LD | BPF_MEM:
		op = "ld"
		operand = fmt.Sprintf("M[%d]", p.K)
	case BPF_LDX | BPF_MEM:
		op = "ldx"
		operand = fmt.Sprintf("M[%d]", p.K)

	case BPF_ST:
		op = "st"
		operand = fmt.Sprintf("M[%d]", p.K)
	case BPF_STX:
		op = "stx"
		operand = fmt.Sprintf("M[%d]", p.K)

	case BPF_JMP | BPF_JA:
		op = "ja"
		operand = strconv.Itoa(n + 1 + int(p.K))

	case BPF_JMP | BPF_JGT | BPF_K:
		op = "jgt"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_JMP | BPF_JGE | BPF_K:
		op = "jge"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_JMP | BPF_JEQ | BPF_K:
		op = "jeq"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_JMP | BPF_JSET | BPF_K:
		op = "jset"
		operand = fmt.Sprintf("#0x%x", p.K)

	case BPF_JMP | BPF_JGT | BPF_X:
		op = "jgt"
		operand = "x"
	case BPF_JMP | BPF_JGE | BPF_X:
		op = "jge"
		operand = "x"
	case BPF_JMP | BPF_JEQ | BPF_X:
		op = "jeq"
		operand = "x"
	case BPF_JMP | BPF_JSET | BPF_X:
		op = "jset"
		operand = "x"

	case BPF_ALU | BPF_ADD | BPF_X:
		op = "add"
		operand = "x"
	case BPF_ALU | BPF_SUB | BPF_X:
		op = "sub"
		operand = "x"
	case BPF_ALU | BPF_MUL | BPF_X:
		op = "mul"
		operand = "x"
	case BPF_ALU | BPF_DIV | BPF_X:
		op = "div"
		operand = "x"
	case BPF_ALU | BPF_MOD | BPF_X:
		op = "mod"
		operand = "x"
	case BPF_ALU | BPF_AND | BPF_X:
		op = "and"
		operand = "x"
	case BPF_ALU | BPF_OR | BPF_X:
		op = "or"
		operand = "x"
	case BPF_ALU | BPF_XOR | BPF_X:
		op = "xor"
		operand = "x"
	case BPF_ALU | BPF_LSH | BPF_X:
		op = "lsh"
		operand = "x"
	case BPF_ALU | BPF_RSH | BPF_X:
		op = "rsh"
		operand = "x"

	case BPF_ALU | BPF_ADD | BPF_K:
		op = "add"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_ALU | BPF_SUB | BPF_K:
		op = "sub"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_ALU | BPF_MUL | BPF_K:
		op = "mul"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_ALU | BPF_DIV | BPF_K:
		op = "div"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_ALU | BPF_MOD | BPF_K:
		op = "mod"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_ALU | BPF_AND | BPF_K:
		op = "and"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_ALU | BPF_OR | BPF_K:
		op = "or"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_ALU | BPF_XOR | BPF_K:
		op = "xor"
		operand = fmt.Sprintf("#0x%x", p.K)
	case BPF_ALU | BPF_LSH | BPF_K:
		op = "lsh"
		operand = fmt.Sprintf("#%d", p.K)
	case BPF_ALU | BPF_RSH | BPF_K:
		op = "rsh"
		operand = fmt.Sprintf("#%d", p.K)

	case BPF_ALU | BPF_NEG:
		op = "neg"
	case BPF_MISC | BPF_TAX:
		op = "tax"
	case BPF_MISC | BPF_TXA:
		op = "txa"
	}

	if Class(p.Code) == BPF_JMP && Op(p.Code) != BPF_JA {
		return fmt.Sprintf("(%03d) %-8s %-16s jt %d\tjf %d",
			n, op, operand, n+1+int(p.Jt), n+1+int(p.Jf))
	}
	if operand == "" {
		return fmt.Sprintf("(%03d) %s", n, op)
	}
	return fmt.Sprintf("(%03d) %-8s %s", n, op, operand)
}
