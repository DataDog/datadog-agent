// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const captureSize = 262144

func generateBPFFilter(c *config.Config) ([]bpf.RawInstruction, error) {
	srcPorts := c.DNSMonitoringPortList
	var dstPorts []int
	if c.CollectDNSStats {
		dstPorts = c.DNSMonitoringPortList
	}

	ins := buildBPFProgram(srcPorts, dstPorts)
	return bpf.Assemble(ins)
}

func buildBPFProgram(srcPorts, dstPorts []int) []bpf.Instruction {
	// Build a BPF filter program that captures DNS traffic on configured ports.
	// Structure:
	// 0: load ethertype
	// 1: if IPv6, goto IPv6 block; else goto IPv4 check
	// 2 to 2+ipv6BlockSize-1: IPv6 block
	// 2+ipv6BlockSize: if IPv4, goto IPv4 block; else drop
	// ... : IPv4 block
	// captureIdx: capture
	// dropIdx: drop

	var ins []bpf.Instruction

	nSrc := len(srcPorts)
	nDst := len(dstPorts)
	hasDst := nDst > 0

	// Port check size: load src + checks + [load dst + checks]
	portChecksSize := 1 + nSrc
	if hasDst {
		portChecksSize += 1 + nDst
	}

	// IPv6 block: header check (1) + TCP check (1) + UDP check (1) + port checks
	ipv6BlockSize := 3 + portChecksSize

	// IPv4 block: header check (1) + TCP check (1) + UDP check (1) + frag load (1) + frag check (1) + memshift (1) + port checks
	ipv4BlockSize := 6 + portChecksSize

	captureIdx := 2 + ipv6BlockSize + 1 + ipv4BlockSize
	dropIdx := captureIdx + 1

	// (0) ldh [12] -- load Ethertype
	ins = append(ins, bpf.LoadAbsolute{Size: 2, Off: 12})

	// (1) jeq #0x86dd jt IPv6Block jf ipv4Check
	ipv4CheckIdx := 2 + ipv6BlockSize
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpEqual,
		Val:       0x86dd,
		SkipTrue:  0,
		SkipFalse: uint8(ipv4CheckIdx - len(ins)),
	})

	// IPv6 block
	// ldb [20] -- load IPv6 Next Header
	ins = append(ins, bpf.LoadAbsolute{Size: 1, Off: 20})

	// jeq #0x6 jt +1 jf 0 -- if TCP, skip UDP check
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpEqual,
		Val:       0x6,
		SkipTrue:  1,
		SkipFalse: 0,
	})

	// jeq #0x11 jt portChecks jf drop
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpEqual,
		Val:       0x11,
		SkipTrue:  0,
		SkipFalse: uint8(dropIdx - len(ins) - 1),
	})

	// IPv6 port checks
	ins = append(ins, buildPortChecks(
		bpf.LoadAbsolute{Size: 2, Off: 54},
		bpf.LoadAbsolute{Size: 2, Off: 56},
		srcPorts, dstPorts,
		captureIdx, dropIdx, len(ins),
	)...)

	// IPv4 check
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpEqual,
		Val:       0x800,
		SkipTrue:  0,
		SkipFalse: uint8(dropIdx - len(ins) - 1),
	})

	// IPv4 block
	// ldb [23] -- load IPv4 Protocol
	ins = append(ins, bpf.LoadAbsolute{Size: 1, Off: 23})

	// jeq #0x6 jt +1 jf 0 -- if TCP, skip UDP check
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpEqual,
		Val:       0x6,
		SkipTrue:  1,
		SkipFalse: 0,
	})

	// jeq #0x11 jt +0 jf drop
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpEqual,
		Val:       0x11,
		SkipTrue:  0,
		SkipFalse: uint8(dropIdx - len(ins) - 1),
	})

	// ldh [20] -- Fragment Offset
	ins = append(ins, bpf.LoadAbsolute{Size: 2, Off: 20})

	// jset #0x1fff jt drop jf 0
	ins = append(ins, bpf.JumpIf{
		Cond:      bpf.JumpBitsSet,
		Val:       0x1fff,
		SkipTrue:  uint8(dropIdx - len(ins) - 1),
		SkipFalse: 0,
	})

	// ldxb 4*([14]&0xf) -- x = IP header length
	ins = append(ins, bpf.LoadMemShift{Off: 14})

	// IPv4 port checks
	ins = append(ins, buildPortChecks(
		bpf.LoadIndirect{Size: 2, Off: 14},
		bpf.LoadIndirect{Size: 2, Off: 16},
		srcPorts, dstPorts,
		captureIdx, dropIdx, len(ins),
	)...)

	// Capture
	ins = append(ins, bpf.RetConstant{Val: captureSize})

	// Drop
	ins = append(ins, bpf.RetConstant{Val: 0})

	return ins
}

func buildPortChecks(srcLoad, dstLoad bpf.Instruction, srcPorts, dstPorts []int, captureIdx, dropIdx, startIdx int) []bpf.Instruction {
	var out []bpf.Instruction

	nDst := len(dstPorts)
	hasDst := nDst > 0

	out = append(out, srcLoad)
	currentIdx := startIdx + 1

	for _, p := range srcPorts {
		skipToCapture := captureIdx - currentIdx - 1
		out = append(out, bpf.JumpIf{
			Cond:      bpf.JumpEqual,
			Val:       uint32(p),
			SkipTrue:  uint8(skipToCapture),
			SkipFalse: 0,
		})
		currentIdx++
	}

	if hasDst {
		out = append(out, dstLoad)
		currentIdx++

		for i, p := range dstPorts {
			skipToCapture := captureIdx - currentIdx - 1
			var skipFalse uint8
			if i == nDst-1 {
				skipFalse = uint8(dropIdx - currentIdx - 1)
			}
			out = append(out, bpf.JumpIf{
				Cond:      bpf.JumpEqual,
				Val:       uint32(p),
				SkipTrue:  uint8(skipToCapture),
				SkipFalse: skipFalse,
			})
			currentIdx++
		}
	} else {
		if len(out) > 1 {
			lastIdx := len(out) - 1
			lastJump := out[lastIdx].(bpf.JumpIf)
			lastJump.SkipFalse = uint8(dropIdx - (startIdx + lastIdx) - 1)
			out[lastIdx] = lastJump
		}
	}

	return out
}
