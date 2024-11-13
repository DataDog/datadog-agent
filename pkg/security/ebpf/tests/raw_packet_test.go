// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/safchain/baloum/pkg/baloum"
)

func testRawPacketFilter(t *testing.T, rawPacketFilters []probes.RawPacketFilter, expectedRetCode int, opts probes.RawPacketProgOpts) {
	var ctx baloum.StdContext

	vm := newVM(t)

	rawPacketEventMap, err := vm.LoadMap("raw_packet_event")
	if err != nil {
		t.Fatal("map not found")
	}
	routerMap, err := vm.LoadMap("classifier_router")
	if err != nil {
		t.Fatal("map not found")
	}

	progSpecs, err := probes.RawPacketTCFiltersToProgramSpecs(rawPacketEventMap.FD(), routerMap.FD(), rawPacketFilters, opts)
	if err != nil {
		t.Fatal(err)
	}

	for i, progSpec := range progSpecs {
		fd := vm.AddProgram(progSpec)

		if _, err := routerMap.Update(probes.TCRawPacketFilterKey+uint32(i), fd, baloum.BPF_ANY); err != nil {
			t.Error(err)
		}
	}

	// override the TCRawPacketParserSenderKey program with a test program
	sendProgSpec := ebpf.ProgramSpec{
		Type: ebpf.SchedCLS,
		Instructions: asm.Instructions{
			asm.Mov.Imm(asm.R0, 2), // put 2 as a success return value
			asm.Return(),
		},
		License: "GPL",
	}
	sendProgFD := vm.AddProgram(&sendProgSpec)

	if _, err := routerMap.Update(probes.TCRawPacketParserSenderKey, sendProgFD, baloum.BPF_ANY); err != nil {
		t.Error(err)
	}

	code, err := vm.RunProgram(&ctx, "test/raw_packet_tail_calls", ebpf.SchedCLS)
	if err != nil || code != expectedRetCode {
		t.Errorf("unexpected error: %v, %d vs %d", err, code, expectedRetCode)
	}
}

func TestRawPacketTailCalls(t *testing.T) {
	t.Run("syn-port-std-ok", func(t *testing.T) {
		filters := []probes.RawPacketFilter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}
		testRawPacketFilter(t, filters, 2, probes.DefaultRawPacketProgOpts)
	})

	t.Run("syn-port-std-ko", func(t *testing.T) {
		filters := []probes.RawPacketFilter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port 6666 and tcp[tcpflags] == tcp-syn",
			},
		}
		testRawPacketFilter(t, filters, 0, probes.DefaultRawPacketProgOpts)
	})

	t.Run("syn-port-mult-ok", func(t *testing.T) {
		filters := []probes.RawPacketFilter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port 6666 and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := probes.DefaultRawPacketProgOpts
		opts.NopInstLen = opts.MaxProgSize

		testRawPacketFilter(t, filters, 2, opts)
	})

	t.Run("syn-port-mult-ko", func(t *testing.T) {
		filters := []probes.RawPacketFilter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port 6666 and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 7777 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := probes.DefaultRawPacketProgOpts
		opts.NopInstLen = opts.MaxProgSize

		testRawPacketFilter(t, filters, 0, opts)
	})
}
