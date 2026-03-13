// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

// Package rawpacket holds rawpacket related files
package rawpacket

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

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
)

const (
	// progPrefix prefix used for raw packet filter programs
	defaultProgPrefix = "raw_packet_filter_"

	// packetCaptureSize see kernel definition
	packetCaptureSize = 256

	// raw packet data, see kernel definition
	// pahole /opt/datadog-agent/embedded/share/system-probe/ebpf/runtime-security-syscall-wrapper.o -y raw_packet_event_t -E --structs -V
	structRawPacketEventPidOffset      = 16
	structRawPacketEventCgroupIdOffset = 80
	structRawPacketEventDataOffset     = 108

	// payload size
	structRawPacketEventDataSize = 256
)

// ProgOpts defines options
type ProgOpts struct {
	*cbpfc.EBPFOpts

	// MaxTailCalls maximun number of tail calls generated
	MaxTailCalls int
	// number of instructions
	MaxProgSize int
	// Number of nop instruction inserted in each program
	NopInstLen int
	// ProgPrefix prefix used for raw packet filter programs
	ProgPrefix string

	// internals
	eventPtrReg           asm.Register
	onMatchLabel          string
	ctxSaveReg            asm.Register
	tailCallMapFd         int
	hasGetCurrentCgroupId bool
}

// DefaultProgOpts default options
func DefaultProgOpts() ProgOpts {
	return ProgOpts{
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
		eventPtrReg:  asm.R8,
		onMatchLabel: "on_match",
		ctxSaveReg:   asm.R9,
		MaxTailCalls: probes.RawPacketMaxTailCall,
		MaxProgSize:  4000,
	}
}

// WithAction sets the action to take when a filter matches
func (opts *ProgOpts) WithProgPrefix(prefix string) *ProgOpts {
	opts.ProgPrefix = prefix
	return opts
}

// WithGetCurrentCgroupID sets if the program should use the get_current_cgroup_id function
func (opts *ProgOpts) WithGetCurrentCgroupID(hasGetCurrentCgroupId bool) *ProgOpts {
	opts.hasGetCurrentCgroupId = hasGetCurrentCgroupId
	return opts
}

// FilterToInsts compile a bpf filter expression
func FilterToInsts(index int, filter Filter, opts ProgOpts) (asm.Instructions, error) {
	pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, 256, filter.BPFFilter)
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
	for i := 0; i != opts.NopInstLen; i++ {
		// insert a nop instruction
		insts = append(insts,
			asm.JEq.Imm(opts.ctxSaveReg, 0, opts.onMatchLabel).WithSymbol(resultLabel),
		)
		resultLabel = ""
	}

	mismatchLabel := fmt.Sprintf("mismatch_%d_", index)

	if filter.Pid != 0 {
		insts = append(insts,
			// == 0, no match
			asm.JEq.Imm(cbpfcOpts.Result, 0, mismatchLabel).WithSymbol(resultLabel),

			// check the pid
			// load the pid from the packet
			asm.LoadMem(asm.R7, opts.eventPtrReg, structRawPacketEventPidOffset, asm.Word),
			asm.JEq.Imm(asm.R7, int32(filter.Pid), opts.onMatchLabel),
			asm.Mov.Imm(asm.R4, 0).WithSymbol(mismatchLabel), // nop instruction, just hold the symbol
		)
	} else if !filter.CGroupPathKey.IsNull() {
		// use the cgroup id which the inode of the cgroup path
		insts = append(insts,
			// == 0, no match
			asm.JEq.Imm(cbpfcOpts.Result, 0, mismatchLabel).WithSymbol(resultLabel),

			// load the cgroup id from the packet
			asm.LoadMem(asm.R7, opts.eventPtrReg, structRawPacketEventCgroupIdOffset, asm.DWord),

			// printk the cgroup id
			/*
				asm.Mov.Reg(asm.R3, asm.R7),
				asm.LoadImm(asm.R2, 2675202386094219606, asm.DWord),
				asm.StoreMem(asm.RFP, -16, asm.R2, asm.DWord),
				asm.Mov.Imm(asm.R2, 100),
				asm.StoreMem(asm.RFP, -8, asm.R2, asm.Half),
				asm.Mov.Reg(asm.R1, asm.RFP),
				asm.Add.Imm(asm.R1, -16),
				asm.Mov.Imm(asm.R2, 10),
				asm.FnTracePrintk.Call(),
			*/

			// check the cgroup id
			asm.LoadImm(asm.R8, int64(filter.CGroupPathKey.Inode), asm.DWord),
			asm.JEq.Reg(asm.R7, asm.R8, opts.onMatchLabel),
			asm.Mov.Imm(asm.R4, 0).WithSymbol(mismatchLabel), // nop instruction, just hold the symbol
		)
	} else {
		insts = append(insts,
			asm.JNE.Imm(cbpfcOpts.Result, 0, opts.onMatchLabel).WithSymbol(resultLabel),
		)
	}
	return insts, nil
}

func filtersToProgs(filters []Filter, opts ProgOpts, headerInsts, footerInsts asm.Instructions) ([]asm.Instructions, *multierror.Error) {
	var (
		progInsts []asm.Instructions
		mErr      *multierror.Error
		tailCalls int
		header    bool
	)

	isMaxSizeExceeded := func(filterInsts, tailCallInsts asm.Instructions) bool {
		return len(filterInsts)+len(tailCallInsts)+len(footerInsts) > opts.MaxProgSize
	}

	for i, filter := range filters {
		filterInsts, err := FilterToInsts(i, filter, opts)
		if err != nil {
			mErr = multierror.Append(mErr, fmt.Errorf("unable to generate eBPF bytecode for rule `%s`: %s", filter.RuleID, err))
			continue
		}

		var tailCallInsts asm.Instructions

		// insert tail call to the current filter if not the last prog
		if i+1 < len(filters) {
			tailCallInsts = asm.Instructions{
				asm.Mov.Reg(asm.R1, opts.ctxSaveReg),
				asm.LoadMapPtr(asm.R2, opts.tailCallMapFd),
				asm.Mov.Imm(asm.R3, int32(probes.TCRawPacketFilterKey+uint32(tailCalls)+1)),
				asm.FnTailCall.Call(),
			}
		}

		// single program exceeded the limit
		if isMaxSizeExceeded(filterInsts, tailCallInsts) {
			mErr = multierror.Append(mErr, fmt.Errorf("max number of intructions exceeded for rule `%s`", filter.RuleID))
			continue
		}

		if !header {
			progInsts = append(progInsts, headerInsts)
			header = true
		}
		progInsts[tailCalls] = append(progInsts[tailCalls], filterInsts...)

		// max size exceeded, generate a new tail call
		if isMaxSizeExceeded(progInsts[tailCalls], tailCallInsts) {
			if opts.MaxTailCalls != 0 && tailCalls >= opts.MaxTailCalls {
				mErr = multierror.Append(mErr, fmt.Errorf("maximum allowed tail calls reach: %d vs %d", tailCalls, opts.MaxTailCalls))
				break
			}

			// insert tail call to the current filter if not the last prog
			progInsts[tailCalls] = append(progInsts[tailCalls], tailCallInsts...)

			// insert the event sender instructions
			progInsts[tailCalls] = append(progInsts[tailCalls], footerInsts...)

			// start a new program
			header = false
			tailCalls++
		}
	}

	if tailCalls < len(progInsts) && header {
		progInsts[tailCalls] = append(progInsts[tailCalls], footerInsts...)
	}

	return progInsts, mErr
}

func getHeaderInsts(rawPacketEventMapFd int, opts ProgOpts) asm.Instructions {
	return append(asm.Instructions{},
		// save ctx
		asm.Mov.Reg(opts.ctxSaveReg, asm.R1),
		// load raw event
		asm.Mov.Reg(asm.R2, asm.RFP),
		asm.Add.Imm(asm.R2, -4),
		asm.StoreImm(asm.R2, 0, 0, asm.Word), // index 0
		asm.LoadMapPtr(asm.R1, rawPacketEventMapFd),
		asm.FnMapLookupElem.Call(),
		asm.JNE.Imm(asm.R0, 0, "raw-packet-event-not-null"),
		asm.Mov.Imm(asm.R0, probes.TCActUnspec),
		asm.Return(),
		// keep the event pointer in the target register
		asm.Mov.Reg(opts.eventPtrReg, asm.R0).WithSymbol("raw-packet-event-not-null"),
		// place in result in the start register and end register
		asm.Mov.Reg(opts.PacketStart, asm.R0),
		asm.Add.Imm(opts.PacketStart, structRawPacketEventDataOffset),
		asm.Mov.Reg(opts.PacketEnd, opts.PacketStart),
		asm.Add.Imm(opts.PacketEnd, structRawPacketEventDataSize),
	)
}

// FiltersToProgramSpecs returns list of program spec from raw packet filters definitions
func FiltersToProgramSpecs(rawPacketEventMapFd, clsRouterMapFd int, filters []Filter, opts ProgOpts) ([]*ebpf.ProgramSpec, error) {
	var mErr *multierror.Error

	if opts.ProgPrefix == "" {
		opts.ProgPrefix = defaultProgPrefix
	}

	opts.tailCallMapFd = clsRouterMapFd

	headerInsts := getHeaderInsts(rawPacketEventMapFd, opts)

	senderInsts := asm.Instructions{
		asm.Mov.Reg(asm.R1, opts.ctxSaveReg).WithSymbol(opts.onMatchLabel),
		asm.LoadMapPtr(asm.R2, clsRouterMapFd),
		asm.Mov.Imm(asm.R3, int32(probes.TCRawPacketSenderKey)),
		asm.FnTailCall.Call(),
		asm.Mov.Imm(asm.R0, probes.TCActUnspec),
		asm.Return(),
	}

	// prepend a return instruction in case of fail
	footerInsts := append(asm.Instructions{
		asm.Mov.Imm(asm.R0, int32(TCActUnspec)),
		asm.Return(),
	}, senderInsts...)

	// compile and convert to eBPF progs
	progInsts, err := filtersToProgs(filters, opts, headerInsts, footerInsts)
	if err.ErrorOrNil() != nil {
		mErr = multierror.Append(mErr, err)
	}

	// should be possible
	if len(progInsts) == 0 {
		return nil, errors.New("no program were generated")
	}

	progSpecs := make([]*ebpf.ProgramSpec, len(progInsts))

	for i, insts := range progInsts {
		name := fmt.Sprintf("%s%d", opts.ProgPrefix, i)

		progSpecs[i] = &ebpf.ProgramSpec{
			Name:         name,
			Type:         ebpf.SchedCLS,
			Instructions: insts,
			License:      "GPL",
		}
	}

	return progSpecs, mErr.ErrorOrNil()
}

// DropActionsToProgramSpecs returns list of program spec from raw packet filters definitions
func DropActionsToProgramSpecs(rawPacketEventMapFd, clsRouterMapFd int, filters []Filter, opts ProgOpts) ([]*ebpf.ProgramSpec, error) {
	var mErr *multierror.Error

	if opts.ProgPrefix == "" {
		opts.ProgPrefix = defaultProgPrefix
	}

	opts.tailCallMapFd = clsRouterMapFd

	headerInsts := getHeaderInsts(rawPacketEventMapFd, opts)

	shotInsts := asm.Instructions{
		asm.Mov.Reg(asm.R1, opts.ctxSaveReg).WithSymbol(opts.onMatchLabel),
		asm.LoadMapPtr(asm.R2, clsRouterMapFd),
		asm.Mov.Imm(asm.R3, int32(probes.TCRawPacketDropActionShotKey)),
		asm.FnTailCall.Call(),
		asm.Mov.Imm(asm.R0, int32(TCActUnspec)),
		asm.Return(),
	}

	// prepend a return instruction in case of fail
	footerInsts := append(asm.Instructions{
		// chain with regular filter
		asm.Mov.Reg(asm.R1, opts.ctxSaveReg),
		asm.LoadMapPtr(asm.R2, clsRouterMapFd),
		asm.Mov.Imm(asm.R3, int32(probes.TCRawPacketFilterKey)),
		asm.FnTailCall.Call(),
		// otherwise accept the packet
		asm.Mov.Imm(asm.R0, int32(TCActUnspec)),
		asm.Return(),
	}, shotInsts...)

	// compile and convert to eBPF progs
	progInsts, err := filtersToProgs(filters, opts, headerInsts, footerInsts)
	if err.ErrorOrNil() != nil {
		mErr = multierror.Append(mErr, err)
	}

	// should be possible
	if len(progInsts) == 0 {
		return nil, errors.New("no program were generated")
	}

	progSpecs := make([]*ebpf.ProgramSpec, len(progInsts))

	for i, insts := range progInsts {
		name := fmt.Sprintf("%s%d", opts.ProgPrefix, i)

		progSpecs[i] = &ebpf.ProgramSpec{
			Name:         name,
			Type:         ebpf.SchedCLS,
			Instructions: insts,
			License:      "GPL",
		}
	}

	return progSpecs, mErr.ErrorOrNil()
}
