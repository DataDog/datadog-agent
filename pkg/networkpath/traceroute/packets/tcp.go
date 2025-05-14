// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"golang.org/x/net/bpf"
)

// TCP4FilterConfig is the config for GenerateTCP4Filter
type TCP4FilterConfig struct {
	Src netip.AddrPort
	Dst netip.AddrPort
}

const ethHeaderSize = 14

// GenerateTCP4Filter creates a classic BPF filter for TCP SOCK_RAW sockets.
// It will only allow packets whose tuple matches the given config.
func (c TCP4FilterConfig) GenerateTCP4Filter() ([]bpf.RawInstruction, error) {
	if !c.Src.Addr().Is4() || !c.Dst.Addr().Is4() {
		return nil, fmt.Errorf("GenerateTCP4Filter2: src=%s and dst=%s must be IPv4", c.Src.Addr(), c.Dst.Addr())
	}
	srcAddr := binary.BigEndian.Uint32(c.Src.Addr().AsSlice())
	dstAddr := binary.BigEndian.Uint32(c.Dst.Addr().AsSlice())
	srcPort := uint32(c.Src.Port())
	dstPort := uint32(c.Dst.Port())

	// Process to derive the following program:
	// 1. Generate the BPF program with placeholder values:
	//    tcpdump -i eth0 -d 'ip and tcp and src 2.4.6.8 and dst 1.3.5.7 and src port 1234 and dst port 5678'
	// 2. Remove the first two instructions that check the ethernet header, since tcpdump uses AF_PACKET and we do not
	// 3. Subtract the ethernet header size from all LoadAbsolutes
	// 4. Replace the placeholder values with src/dst AddrPorts
	return bpf.Assemble([]bpf.Instruction{
		// (002) ldb      [23] -- load Protocol
		bpf.LoadAbsolute{Size: 1, Off: 23 - ethHeaderSize},
		// (003) jeq      #0x6             jt 4	jf 16 -- if TCP, goto 4, else 16
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x6, SkipTrue: 0, SkipFalse: 12},
		// (004) ld       [26] -- load source IP
		bpf.LoadAbsolute{Size: 4, Off: 26 - ethHeaderSize},
		// (005) jeq      #0x2040608       jt 6	jf 16 -- if srcAddr matches, goto 6, else 16
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: srcAddr, SkipTrue: 0, SkipFalse: 10},
		// (006) ld       [30] -- load destination IP
		bpf.LoadAbsolute{Size: 4, Off: 30 - ethHeaderSize},
		// (007) jeq      #0x1030507       jt 8	jf 16 -- if dstAddr matches, goto 8, else 16
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: dstAddr, SkipTrue: 0, SkipFalse: 8},
		// (008) ldh      [20] -- load Fragment Offset
		bpf.LoadAbsolute{Size: 2, Off: 20 - ethHeaderSize},
		// (009) jset     #0x1fff          jt 16	jf 10 -- if fragmented, goto 16, else 10
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: 6, SkipFalse: 0},
		// (010) ldxb     4*([14]&0xf) -- x = IP header length
		bpf.LoadMemShift{Off: 14 - ethHeaderSize},
		// (011) ldh      [x + 14] -- load source port
		bpf.LoadIndirect{Size: 2, Off: 14 - ethHeaderSize},
		// (012) jeq      #0x4d2           jt 13	jf 16 -- if srcPort matches, goto 13, else 16
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: srcPort, SkipTrue: 0, SkipFalse: 3},
		// (013) ldh      [x + 16] -- load destination port
		bpf.LoadIndirect{Size: 2, Off: 16 - ethHeaderSize},
		// (014) jeq      #0x162e          jt 15	jf 16 -- if dstPort matches, goto 15, else 16
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: dstPort, SkipTrue: 0, SkipFalse: 1},
		// (015) ret      #262144 -- accept packet
		bpf.RetConstant{Val: 262144},
		// (016) ret      #0 -- drop packet
		bpf.RetConstant{Val: 0},
	})
}
