// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"math"

	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const port = 53

func generateBPFFilter(c *config.Config) ([]bpf.RawInstruction, error) {
	allowedDestPort := uint32(math.MaxUint32)
	if c.CollectDNSStats {
		allowedDestPort = port
	}

	return bpf.Assemble([]bpf.Instruction{
		//(000) ldh      [12] -- load Ethertype
		bpf.LoadAbsolute{Size: 2, Off: 12},
		//(001) jeq      #0x86dd          jt 2	jf 9 -- if IPv6, goto 2, else 9
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x86dd, SkipTrue: 0, SkipFalse: 7},
		//(002) ldb      [20] -- load IPv6 Next Header
		bpf.LoadAbsolute{Size: 1, Off: 20},
		//(003) jeq      #0x6             jt 5	jf 4 -- IPv6 Next Header: if TCP, goto 5
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x6, SkipTrue: 1, SkipFalse: 0},
		//(004) jeq      #0x11            jt 5	jf 21 -- IPv6 Next Header: if UDP, goto 5, else drop
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x11, SkipTrue: 0, SkipFalse: 16},
		//(005) ldh      [54] -- load source port
		bpf.LoadAbsolute{Size: 2, Off: 54},
		//(006) jeq      #0x35            jt 20	jf 7 -- if 53, capture
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: port, SkipTrue: 13, SkipFalse: 0},
		//(007) ldh      [56] -- load dest port
		bpf.LoadAbsolute{Size: 2, Off: 56},
		//(008) jeq      #0x35            jt 20	jf 21 -- if allowedDestPort, capture, else drop
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: allowedDestPort, SkipTrue: 11, SkipFalse: 12},
		//(009) jeq      #0x800           jt 10	jf 21 -- if IPv4, go next, else drop
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x800, SkipTrue: 0, SkipFalse: 11},
		//(010) ldb      [23] -- load IPv4 Protocol
		bpf.LoadAbsolute{Size: 1, Off: 23},
		//(011) jeq      #0x6             jt 13	jf 12 -- if TCP, goto 13
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x6, SkipTrue: 1, SkipFalse: 0},
		//(012) jeq      #0x11            jt 13	jf 21 -- if UDP, goto 13, else drop
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x11, SkipTrue: 0, SkipFalse: 8},
		//(013) ldh      [20] -- load Fragment Offset
		bpf.LoadAbsolute{Size: 2, Off: 20},
		//(014) jset     #0x1fff          jt 21	jf 15 -- use 0x1fff as mask for fragment offset, if != 0, drop
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: 6, SkipFalse: 0},
		//(015) ldxb     4*([14]&0xf) -- x = IP header length
		bpf.LoadMemShift{Off: 14},
		//(016) ldh      [x + 14] -- load source port
		bpf.LoadIndirect{Size: 2, Off: 14},
		//(017) jeq      #0x35            jt 20	jf 18 -- if port 53 capture
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: port, SkipTrue: 2, SkipFalse: 0},
		//(018) ldh      [x + 16] -- load dest port
		bpf.LoadIndirect{Size: 2, Off: 16},
		//(019) jeq      #0x35            jt 20	jf 21 -- if port allowedDestPort capture, else drop
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: allowedDestPort, SkipTrue: 0, SkipFalse: 1},
		//(020) ret      #262144 -- capture
		bpf.RetConstant{Val: 262144},
		//(021) ret      #0 -- drop
		bpf.RetConstant{Val: 0},
	})
}
