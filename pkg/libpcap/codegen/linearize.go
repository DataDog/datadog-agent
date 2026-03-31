// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// NOP is a no-op instruction code used during optimization. Statements with
// this code are skipped during linearization.
const NOP = -1

// slength counts the number of non-NOP statements in an SList chain.
func slength(s *SList) uint {
	var n uint
	for ; s != nil; s = s.Next {
		if s.S.Code != NOP {
			n++
		}
	}
	return n
}

// countStmts returns the total number of instructions reachable from block p.
// Includes side-effect statements, the conditional jump, and any extra long jumps.
func countStmts(ic *ICode, p *Block) uint {
	if p == nil || ic.IsMarked(p) {
		return 0
	}
	ic.MarkBlock(p)
	n := countStmts(ic, JT(p)) + countStmts(ic, JF(p))
	return slength(p.Stmts) + n + 1 + uint(p.LongJt) + uint(p.LongJf)
}

// IcodeToFcode converts the block CFG to a flat BPF instruction array.
// This is the linearization step that produces executable BPF bytecode.
// Port of icode_to_fcode() from optimize.c.
func IcodeToFcode(ic *ICode, root *Block) ([]bpf.Instruction, error) {
	if root == nil {
		return nil, fmt.Errorf("empty filter program")
	}

	// Loop doing convertCode until no branches remain with too-large offsets.
	// Each iteration may discover branches that need long jumps, which increases
	// the instruction count and requires re-linearization.
	for iteration := 0; iteration < 100; iteration++ {
		ic.UnMarkAll()
		n := countStmts(ic, root)
		if n == 0 {
			return nil, fmt.Errorf("filter has no instructions")
		}

		fp := make([]bpf.Instruction, n)

		ic.UnMarkAll()
		tailIdx := int(n)
		ok := convertCode(ic, root, fp, &tailIdx)
		if ok {
			return fp, nil
		}
		// Some branch was too large — long jumps have been marked, retry
	}
	return nil, fmt.Errorf("linearization failed: too many iterations")
}

// convertCode recursively converts blocks to BPF instructions.
// It fills the instruction array from the end (tail) backwards.
// Returns true if all branch offsets fit, false if a long jump is needed.
// Port of convert_code_r() from optimize.c.
func convertCode(ic *ICode, p *Block, fp []bpf.Instruction, tailIdx *int) bool {
	if p == nil || ic.IsMarked(p) {
		return true
	}
	ic.MarkBlock(p)

	// Process JF first, then JT (reverse post-order)
	if !convertCode(ic, JF(p), fp, tailIdx) {
		return false
	}
	if !convertCode(ic, JT(p), fp, tailIdx) {
		return false
	}

	slen := slength(p.Stmts)
	totalLen := slen + 1 + uint(p.LongJt) + uint(p.LongJf)

	*tailIdx -= int(totalLen)
	dstIdx := *tailIdx

	p.Offset = dstIdx

	// Emit side-effect statements
	for src := p.Stmts; src != nil; src = src.Next {
		if src.S.Code == NOP {
			continue
		}
		fp[dstIdx] = bpf.Instruction{
			Code: uint16(src.S.Code),
			K:    src.S.K,
		}
		dstIdx++
	}

	// Emit the branch/return instruction
	fp[dstIdx] = bpf.Instruction{
		Code: uint16(p.S.Code),
		K:    p.S.K,
	}

	if JT(p) != nil {
		// Conditional jump — compute branch offsets
		var extraJmps uint8

		// True branch offset
		jtOff := JT(p).Offset - (p.Offset + int(slen)) - 1
		if jtOff >= 256 {
			if p.LongJt == 0 {
				// Mark for long jump and retry
				p.LongJt = 1
				return false
			}
			fp[dstIdx].Jt = extraJmps
			extraJmps++
			dstIdx++
			fp[dstIdx] = bpf.Instruction{
				Code: uint16(bpf.BPF_JMP | bpf.BPF_JA),
				K:    uint32(jtOff - int(extraJmps)),
			}
		} else {
			fp[dstIdx].Jt = uint8(jtOff)
		}

		// False branch offset
		jfOff := JF(p).Offset - (p.Offset + int(slen)) - 1
		if jfOff >= 256 {
			if p.LongJf == 0 {
				p.LongJf = 1
				return false
			}
			fp[dstIdx-int(extraJmps)].Jf = extraJmps
			extraJmps++
			dstIdx++
			fp[dstIdx] = bpf.Instruction{
				Code: uint16(bpf.BPF_JMP | bpf.BPF_JA),
				K:    uint32(jfOff - int(extraJmps)),
			}
		} else {
			fp[dstIdx-int(extraJmps)].Jf = uint8(jfOff)
		}
	}

	return true
}
