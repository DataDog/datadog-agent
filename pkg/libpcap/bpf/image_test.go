// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bpf

import "testing"

func TestImage(t *testing.T) {
	tests := []struct {
		name string
		insn Instruction
		n    int
		want string
	}{
		{
			name: "ret k",
			insn: Stmt(BPF_RET|BPF_K, 65535),
			n:    0,
			want: "(000) ret      #65535",
		},
		{
			name: "ret a",
			insn: Stmt(BPF_RET|BPF_A, 0),
			n:    0,
			want: "(000) ret",
		},
		{
			name: "ld abs word",
			insn: Stmt(BPF_LD|BPF_W|BPF_ABS, 12),
			n:    0,
			want: "(000) ld       [12]",
		},
		{
			name: "ldh abs",
			insn: Stmt(BPF_LD|BPF_H|BPF_ABS, 12),
			n:    0,
			want: "(000) ldh      [12]",
		},
		{
			name: "ldb abs",
			insn: Stmt(BPF_LD|BPF_B|BPF_ABS, 23),
			n:    2,
			want: "(002) ldb      [23]",
		},
		{
			name: "ld ind word",
			insn: Stmt(BPF_LD|BPF_W|BPF_IND, 4),
			n:    1,
			want: "(001) ld       [x + 4]",
		},
		{
			name: "ld imm",
			insn: Stmt(BPF_LD|BPF_IMM, 0x800),
			n:    0,
			want: "(000) ld       #0x800",
		},
		{
			name: "ldx imm",
			insn: Stmt(BPF_LDX|BPF_IMM, 0x10),
			n:    0,
			want: "(000) ldx      #0x10",
		},
		{
			name: "ldxb msh",
			insn: Stmt(BPF_LDX|BPF_MSH|BPF_B, 14),
			n:    0,
			want: "(000) ldxb     4*([14]&0xf)",
		},
		{
			name: "ld mem",
			insn: Stmt(BPF_LD|BPF_MEM, 3),
			n:    0,
			want: "(000) ld       M[3]",
		},
		{
			name: "st",
			insn: Stmt(BPF_ST, 5),
			n:    0,
			want: "(000) st       M[5]",
		},
		{
			name: "stx",
			insn: Stmt(BPF_STX, 2),
			n:    0,
			want: "(000) stx      M[2]",
		},
		{
			name: "ja",
			insn: Stmt(BPF_JMP|BPF_JA, 3),
			n:    0,
			want: "(000) ja       4",
		},
		{
			name: "jeq k",
			insn: Jump(BPF_JMP|BPF_JEQ|BPF_K, 0x800, 0, 3),
			n:    1,
			want: "(001) jeq      #0x800           jt 2\tjf 5",
		},
		{
			name: "jgt x",
			insn: Jump(BPF_JMP|BPF_JGT|BPF_X, 0, 1, 2),
			n:    5,
			want: "(005) jgt      x                jt 7\tjf 8",
		},
		{
			name: "add x",
			insn: Stmt(BPF_ALU|BPF_ADD|BPF_X, 0),
			n:    0,
			want: "(000) add      x",
		},
		{
			name: "add k",
			insn: Stmt(BPF_ALU|BPF_ADD|BPF_K, 14),
			n:    0,
			want: "(000) add      #14",
		},
		{
			name: "and k",
			insn: Stmt(BPF_ALU|BPF_AND|BPF_K, 0xff),
			n:    0,
			want: "(000) and      #0xff",
		},
		{
			name: "neg",
			insn: Stmt(BPF_ALU|BPF_NEG, 0),
			n:    0,
			want: "(000) neg",
		},
		{
			name: "tax",
			insn: Stmt(BPF_MISC|BPF_TAX, 0),
			n:    0,
			want: "(000) tax",
		},
		{
			name: "txa",
			insn: Stmt(BPF_MISC|BPF_TXA, 0),
			n:    0,
			want: "(000) txa",
		},
		{
			name: "ld pktlen",
			insn: Stmt(BPF_LD|BPF_W|BPF_LEN, 0),
			n:    0,
			want: "(000) ld       #pktlen",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Image(tt.insn, tt.n)
			if got != tt.want {
				t.Errorf("Image() = %q, want %q", got, tt.want)
			}
		})
	}
}
