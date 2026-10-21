// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package usdt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func constArg(size int, v int64) ArgSpec {
	return sized(ArgSpec{Arg_type: ArgConst, Val_off: uint64(v)}, size)
}

func regArg(size int, regOff int16) ArgSpec {
	return sized(ArgSpec{Arg_type: ArgReg, Reg_off: regOff}, size)
}

func derefArg(size int, regOff int16, off int64) ArgSpec {
	return sized(ArgSpec{Arg_type: ArgRegDeref, Reg_off: regOff, Val_off: uint64(off)}, size)
}

func sized(a ArgSpec, size int) ArgSpec {
	if size < 0 {
		a.Arg_signed = 1
		size = -size
	}
	a.Arg_bitshift = uint8(64 - 8*size)
	return a
}

func specOf(args ...ArgSpec) Spec {
	var s Spec
	s.Cnt = uint16(copy(s.Args[:], args))
	return s
}

// offsets in struct pt_regs (x86-64)
const (
	x86R15 = 0
	x86R12 = 24
	x86RBP = 32
	x86RBX = 40
	x86R10 = 56
	x86R9  = 64
	x86R8  = 72
	x86RAX = 80
	x86RCX = 88
	x86RDX = 96
	x86RSI = 104
	x86RDI = 112
	x86RSP = 152
)

func TestParseArgsX86(t *testing.T) {
	for _, tc := range []struct {
		args string
		want Spec
	}{
		// from HotSpot builds
		{"1@%dil", specOf(regArg(1, x86RDI))},
		{"1@$1", specOf(constArg(1, 1))},
		{"1@$0", specOf(constArg(1, 0))},
		{"-4@-20(%rbp)", specOf(derefArg(-4, x86RBP, -20))},
		{"8@-120(%rbp)", specOf(derefArg(8, x86RBP, -120))},
		{"8@%rax", specOf(regArg(8, x86RAX))},
		{"8@%r8", specOf(regArg(8, x86R8))},
		{"4@%r10d", specOf(regArg(4, x86R10))},
		{
			"8@%rbx 8@%r12 8@-112(%rbp) 8@%rcx 8@%rdx 8@%rsi 8@%rdi 8@%r9",
			specOf(regArg(8, x86RBX), regArg(8, x86R12), derefArg(8, x86RBP, -112), regArg(8, x86RCX),
				regArg(8, x86RDX), regArg(8, x86RSI), regArg(8, x86RDI), regArg(8, x86R9)),
		},
		// register aliases
		{"8@%rsp 4@%esp 2@%sp 1@%spl", specOf(regArg(8, x86RSP), regArg(4, x86RSP), regArg(2, x86RSP), regArg(1, x86RSP))},
		{"-2@%ax -1@%al -4@%eax", specOf(regArg(-2, x86RAX), regArg(-1, x86RAX), regArg(-4, x86RAX))},
		{"1@%sil 2@%di 4@%ebp 1@%bl", specOf(regArg(1, x86RSI), regArg(2, x86RDI), regArg(4, x86RBP), regArg(1, x86RBX))},
		{"8@%r15 4@%r15d 2@%r15w 1@%r15b", specOf(regArg(8, x86R15), regArg(4, x86R15), regArg(2, x86R15), regArg(1, x86R15))},
		// constants and displacements
		{"-4@$-9", specOf(constArg(-4, -9))},
		{"8@$0x10 8@$+3", specOf(constArg(8, 16), constArg(8, 3))},
		{"8@$18446744073709551615", specOf(constArg(8, -1))},
		{"8@(%rax)", specOf(derefArg(8, x86RAX, 0))},
		{"2@0x18(%rsi)", specOf(derefArg(2, x86RSI, 0x18))},
		{"-8@-0x8(%rsp)", specOf(derefArg(-8, x86RSP, -8))},
		// separators
		{"", Spec{}},
		{"  8@%rax\t\t1@$1  ", specOf(regArg(8, x86RAX), constArg(1, 1))},
		{strings.Repeat("8@%rax ", MaxArgs), specOf(
			regArg(8, x86RAX), regArg(8, x86RAX), regArg(8, x86RAX), regArg(8, x86RAX),
			regArg(8, x86RAX), regArg(8, x86RAX), regArg(8, x86RAX), regArg(8, x86RAX),
			regArg(8, x86RAX), regArg(8, x86RAX), regArg(8, x86RAX), regArg(8, x86RAX))},
	} {
		t.Run(tc.args, func(t *testing.T) {
			spec, err := ParseArgs(safeelf.EM_X86_64, tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.want, spec)
		})
	}
}

func TestParseArgsARM64(t *testing.T) {
	for _, tc := range []struct {
		args string
		want Spec
	}{
		// from HotSpot builds
		{"1@x0", specOf(regArg(1, 0))},
		{"1@1", specOf(constArg(1, 1))},
		{"1@0", specOf(constArg(1, 0))},
		{"8@x1", specOf(regArg(8, 8))},
		{"8@[sp, 112]", specOf(derefArg(8, 248, 112))},
		{
			"8@x20 8@x21 8@[sp, 112] 8@x22 8@x23 8@x24 8@x25 8@x26",
			specOf(regArg(8, 160), regArg(8, 168), derefArg(8, 248, 112), regArg(8, 176),
				regArg(8, 184), regArg(8, 192), regArg(8, 200), regArg(8, 208)),
		},
		// other forms
		{"-4@w2", specOf(regArg(-4, 16))},
		{"8@[x19]", specOf(derefArg(8, 152, 0))},
		{"8@sp", specOf(regArg(8, 248))},
		{"8@x30 4@w30", specOf(regArg(8, 240), regArg(4, 240))},
		{"-4@[x29, -12]", specOf(derefArg(-4, 232, -12))},
		{"2@[sp,#16] 8@[ x3 , 0x8 ]", specOf(derefArg(2, 248, 16), derefArg(8, 24, 8))},
		{"-8@-5 4@#7 8@0x20", specOf(constArg(-8, -5), constArg(4, 7), constArg(8, 0x20))},
		{"", Spec{}},
	} {
		t.Run(tc.args, func(t *testing.T) {
			spec, err := ParseArgs(safeelf.EM_AARCH64, tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.want, spec)
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, tc := range []struct {
		machine safeelf.Machine
		args    string
		err     string
	}{
		{safeelf.EM_X86_64, "8@(%rax,%rbx,8)", "SIB addressing is not supported"},
		{safeelf.EM_X86_64, "-1@-96(%rbp,%rax,8)", "SIB addressing is not supported"},
		{safeelf.EM_X86_64, "3@%rax", "invalid size"},
		{safeelf.EM_X86_64, "16@%rax", "invalid size"},
		{safeelf.EM_X86_64, "0@%rax", "invalid size"},
		{safeelf.EM_X86_64, "x@%rax", "invalid size"},
		{safeelf.EM_X86_64, "%rax", "missing size"},
		{safeelf.EM_X86_64, "8@%rax %rbx", "missing size"},
		{safeelf.EM_X86_64, "8@%foo", `unsupported register "foo"`},
		{safeelf.EM_X86_64, "1@%ah", `unsupported register "ah"`},
		{safeelf.EM_X86_64, "8@%rip", `unsupported register "rip"`},
		{safeelf.EM_X86_64, "8@sym(%rip)", `unsupported register "rip"`},
		{safeelf.EM_X86_64, "8@sym(%rax)", "invalid number"},
		{safeelf.EM_X86_64, "8@sym", "unsupported operand"},
		{safeelf.EM_X86_64, "8@$sym", "invalid number"},
		{safeelf.EM_X86_64, "8@$1_000", "invalid number"},
		{safeelf.EM_X86_64, "8@$0x", "invalid number"},
		{safeelf.EM_X86_64, "8@$--1", "invalid number"},
		{safeelf.EM_X86_64, "8@x1", "unsupported operand"},
		{safeelf.EM_X86_64, "8@", "unsupported operand"},
		{safeelf.EM_X86_64, strings.Repeat("8@%rax ", MaxArgs+1), "more than 12 arguments"},
		{safeelf.EM_AARCH64, "8@[x0, x1]", "register offset addressing is not supported"},
		{safeelf.EM_AARCH64, "8@[w0]", `unsupported register "w0"`},
		{safeelf.EM_AARCH64, "8@[x0", "unsupported operand"},
		{safeelf.EM_AARCH64, "8@x31", `unsupported register "x31"`},
		{safeelf.EM_AARCH64, "8@x01", `unsupported register "x01"`},
		{safeelf.EM_AARCH64, "8@fp", `unsupported register "fp"`},
		{safeelf.EM_AARCH64, "8@%rax", `unsupported register "%rax"`},
		{safeelf.EM_AARCH64, "5@x0", "invalid size"},
		{safeelf.Machine(3), "", "unsupported machine"},       // i386
		{safeelf.Machine(243), "8@a0", "unsupported machine"}, // RISC-V
	} {
		t.Run(tc.args, func(t *testing.T) {
			_, err := ParseArgs(tc.machine, tc.args)
			assert.ErrorContains(t, err, tc.err)
		})
	}
}
