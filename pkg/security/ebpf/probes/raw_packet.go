// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cloudflare/cbpfc"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// RawPacketTCProgram returns the list of TC classifier sections
var RawPacketTCProgram = []string{
	"classifier_raw_packet_egress",
	"classifier_raw_packet_ingress",
}

const (
	// RawPacketFilterProgPrefix prefix used for raw packet filter programs
	RawPacketFilterProgPrefix = "raw_packet_prog_"

	// First raw packet tc program to be called
	RawPacketFilterEntryProg = "raw_packet_prog_0"

	// RawPacketCaptureSize see kernel definition
	RawPacketCaptureSize = 256

	// RawPacketFilterMaxTailCall defines the maximum of tail calls
	RawPacketFilterMaxTailCall = 5
)

// RawPacketProgOpts defines options
type RawPacketProgOpts struct {
	*cbpfc.EBPFOpts
	sendEventLabel string
	ctxSave        asm.Register
	tailCallMapFd  int
	nopInstLen     int
}

// DefaultRawPacketProgOpts default options
var DefaultRawPacketProgOpts = RawPacketProgOpts{
	EBPFOpts: &cbpfc.EBPFOpts{
		PacketStart: asm.R1,
		PacketEnd:   asm.R2,
		Result:      asm.R3,
		Working: [4]asm.Register{
			asm.R4,
			asm.R5,
			asm.R6,
			asm.R7,
		},
		StackOffset: 16, // adapt using the stack size used outside of the filter itself, ex: map_lookup
	},
	sendEventLabel: "send_event",
	ctxSave:        asm.R9,
}

// BPFFilterToInsts compile a bpf filter expression
func BPFFilterToInsts(index int, filter string, opts RawPacketProgOpts) (asm.Instructions, error) {
	pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, 256, filter)
	if err != nil {
		return nil, err
	}
	bpfInsts := make([]bpf.Instruction, len(pcapBPF))
	for i, ri := range pcapBPF {
		bpfInsts[i] = bpf.RawInstruction{Op: ri.Code, Jt: ri.Jt, Jf: ri.Jf, K: ri.K}.Disassemble()
	}

	var cbpfcOpts cbpfc.EBPFOpts
	if opts.EBPFOpts != nil {
		// make a copy so that we can modify the labels
		cbpfcOpts = *opts.EBPFOpts
	}
	cbpfcOpts.LabelPrefix = fmt.Sprintf("cbpfc_%d_", index)
	cbpfcOpts.ResultLabel = fmt.Sprintf("check_result_%d", index)

	insts, err := cbpfc.ToEBPF(bpfInsts, cbpfcOpts)
	if err != nil {
		return nil, err
	}

	resultLabel := cbpfcOpts.ResultLabel

	// add nop insts, used to test the max insts and artificially generate tail calls
	for i := 0; i != opts.nopInstLen; i++ {
		insts = append(insts,
			asm.JEq.Imm(asm.R9, 0, opts.sendEventLabel).WithSymbol(resultLabel),
		)
		resultLabel = ""
	}

	// filter result
	insts = append(insts,
		asm.JNE.Imm(cbpfcOpts.Result, 0, opts.sendEventLabel).WithSymbol(resultLabel),
	)

	return insts, nil
}

func rawPacketFiltersToProgs(rawPacketfilters []RawPacketFilter, opts RawPacketProgOpts, headerInsts, senderInsts asm.Instructions) ([]asm.Instructions, *multierror.Error) {
	var (
		progInsts   []asm.Instructions
		currProg    uint32
		maxProgSize = 4000
		mErr        *multierror.Error
	)

	progInsts = append(progInsts, asm.Instructions{})

	progInsts[currProg] = append(progInsts[currProg], headerInsts...)

	for i, rawPacketFilter := range rawPacketfilters {
		filterInsts, err := BPFFilterToInsts(i, rawPacketFilter.BPFFilter, opts)
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to generate eBPF bitcode for rule `%s`: %s", rawPacketFilter.RuleID, err))
			continue
		}

		tailCallInsts := asm.Instructions{
			asm.Mov.Reg(asm.R1, opts.ctxSave),
			asm.LoadMapPtr(asm.R2, opts.tailCallMapFd),
			asm.Mov.Imm(asm.R3, int32(TCRawPacketFilterKey+currProg+1)),
			asm.FnTailCall.Call(),
		}

		// max size exceeded, generate a new tail call
		if len(progInsts[currProg])+len(tailCallInsts)+len(senderInsts) > maxProgSize {
			// insert tail call to the previews filter
			progInsts[currProg] = append(progInsts[currProg], tailCallInsts...)
			progInsts[currProg] = append(progInsts[currProg], senderInsts...)
			currProg++

			// start a new program
			progInsts = append(progInsts, asm.Instructions{})
			progInsts[currProg] = append(progInsts[currProg], headerInsts...)
		}
		progInsts[currProg] = append(progInsts[currProg], filterInsts...)
	}

	progInsts[currProg] = append(progInsts[currProg],
		asm.Return(),
	)
	progInsts[currProg] = append(progInsts[currProg], senderInsts...)

	return progInsts, mErr
}

// RawPacketFilter defines a raw packet filter
type RawPacketFilter struct {
	RuleID    eval.RuleID
	BPFFilter string
}

// RawPacketTCFiltersToCollectionSpec returns a collection spec from raw packet filters definitions
func RawPacketTCFiltersToCollectionSpec(rawPacketEventMapFd, clsRouterMapFd int, rawpPacketFilters []RawPacketFilter) (*ebpf.CollectionSpec, error) {
	var mErr *multierror.Error

	const (
		// raw packet data, see kernel definition
		dataSize   = 256
		dataOffset = 164
	)

	opts := DefaultRawPacketProgOpts
	opts.tailCallMapFd = clsRouterMapFd

	headerInsts := append(asm.Instructions{},
		// save ctx
		asm.Mov.Reg(opts.ctxSave, asm.R1),
		// load raw event
		asm.Mov.Reg(asm.R2, asm.RFP),
		asm.Add.Imm(asm.R2, -4),
		asm.StoreImm(asm.R2, 0, 0, asm.Word), // index 0
		asm.LoadMapPtr(asm.R1, rawPacketEventMapFd),
		asm.FnMapLookupElem.Call(),
		asm.JNE.Imm(asm.R0, 0, "raw-packet-event-not-null"),
		asm.Return(),
		// place in result in the start register and end register
		asm.Mov.Reg(opts.PacketStart, asm.R0).WithSymbol("raw-packet-event-not-null"),
		asm.Add.Imm(opts.PacketStart, dataOffset),
		asm.Mov.Reg(opts.PacketEnd, opts.PacketStart),
		asm.Add.Imm(opts.PacketEnd, dataSize),
	)

	senderInsts := asm.Instructions{
		asm.Mov.Reg(asm.R1, opts.ctxSave).WithSymbol(opts.sendEventLabel),
		asm.LoadMapPtr(asm.R2, clsRouterMapFd),
		asm.Mov.Imm(asm.R3, int32(TCRawPacketParserSenderKey)),
		asm.FnTailCall.Call(),
		asm.Mov.Imm(asm.R0, 0),
		asm.Return(),
	}

	// compile and convert to eBPF progs
	progs, err := rawPacketFiltersToProgs(rawpPacketFilters, opts, headerInsts, senderInsts)
	if err.ErrorOrNil() != nil {
		mErr = multierror.Append(mErr, err)
	}

	// should be possible
	if len(progs) == 0 {
		return nil, errors.New("no program were generated")
	}

	// entry program and maybe the only one
	colSpec := &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			RawPacketFilterEntryProg: {
				Type:         ebpf.SchedCLS,
				Instructions: progs[0],
				License:      "GPL",
			},
		},
	}

	for i, insts := range progs[1:] {
		name := fmt.Sprintf("%s_%d", RawPacketFilterProgPrefix, i+1)
		colSpec.Programs[name] = &ebpf.ProgramSpec{
			Type:         ebpf.SchedCLS,
			Instructions: insts,
			License:      "GPL",
		}
	}

	return colSpec, mErr.ErrorOrNil()
}
