// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package rules

import (
	"errors"

	"github.com/cilium/ebpf/asm"
	"github.com/cloudflare/cbpfc"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/bpf"
)

func init() {
	DefaultValidateBPFFilter = validateNetworkFilterBPFFilter
}

func validateNetworkFilterBPFFilter(bpfFilter string) error {
	pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, 256, bpfFilter)
	if err != nil {
		return errors.New("a valid BPF filter must be specified to the 'network_filter' action, error: " + err.Error())
	}

	bpfInsts := make([]bpf.Instruction, len(pcapBPF))
	for i, ri := range pcapBPF {
		bpfInsts[i] = bpf.RawInstruction{Op: ri.Code, Jt: ri.Jt, Jf: ri.Jf, K: ri.K}.Disassemble()
	}

	_, err = cbpfc.ToEBPF(bpfInsts, cbpfc.EBPFOpts{
		PacketStart: asm.R1,
		PacketEnd:   asm.R2,
		Result:      asm.R3,
		Working: [4]asm.Register{
			asm.R4,
			asm.R5,
			asm.R6,
			asm.R7,
		},
		StackOffset: 16,
		LabelPrefix: "cbpfc_validate_",
		ResultLabel: "check_result_validate",
	})
	if err != nil {
		return errors.New("a valid BPF filter must be specified to the 'network_filter' action, error: " + err.Error())
	}

	return nil
}
