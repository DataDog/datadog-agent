// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata && pcap && cgo

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/safchain/baloum/pkg/baloum"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
)

func testRawPacketFilter(t *testing.T, filters []rawpacket.Filter, progName string, expRetCode int64, expProgNum int, opts rawpacket.ProgOpts, catchCompilerError bool) {
	var ctx baloum.StdContext

	vm := newVM(t)

	rawPacketEventMap, err := vm.LoadMap("raw_packet_event")
	assert.Nil(t, err, "map not found")

	routerMap, err := vm.LoadMap("raw_packet_classifier_router")
	assert.Nil(t, err, "map not found")

	progSpecs, err := rawpacket.FiltersToProgramSpecs(rawPacketEventMap.FD(), routerMap.FD(), filters, opts)
	if err != nil {
		if catchCompilerError {
			t.Fatal(err)
		} else {
			t.Log(err)
		}
	}

	assert.Equal(t, expProgNum, len(progSpecs), "number of expected programs")

	for i, progSpec := range progSpecs {
		fd := vm.AddProgram(progSpec)

		_, err := routerMap.Update(probes.TCRawPacketFilterKey+uint32(i), fd, baloum.BPF_ANY)
		assert.Nil(t, err, "map update error")
	}

	// override the TCRawPacketParserSenderKey program with a test program
	sendProgSpec := ebpf.ProgramSpec{
		Type: ebpf.SchedCLS,
		Instructions: asm.Instructions{
			asm.Mov.Imm(asm.R0, 255), // put 2 as a success return value
			asm.Return(),
		},
		License: "GPL",
	}
	sendProgFD := vm.AddProgram(&sendProgSpec)

	_, err = routerMap.Update(probes.TCRawPacketParserSenderKey, sendProgFD, baloum.BPF_ANY)
	assert.Nil(t, err, "map update error")

	code, err := vm.RunProgram(&ctx, progName, ebpf.SchedCLS)
	if expRetCode != -1 {
		assert.Nil(t, err, "program execution error")
	}
	assert.Equal(t, expRetCode, code, "return code error: %v", err)
}

func TestRawPacketTailCalls(t *testing.T) {
	t.Run("syn-port-std-ok", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}
		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", 255, 1, rawpacket.DefaultProgOpts(), true)
	})

	t.Run("syn-port-std-ko", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port 6666 and tcp[tcpflags] == tcp-syn",
			},
		}
		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", probes.TCActUnspec, 1, rawpacket.DefaultProgOpts(), true)
	})

	t.Run("syn-port-std-limit-ko", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := rawpacket.DefaultProgOpts()
		opts.NopInstLen = opts.MaxProgSize

		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", -1, 0, opts, false)
	})

	t.Run("syn-port-std-syntax-err", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port number and tcp[tcpflags] == tcp-syn",
			},
		}
		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", -1, 0, rawpacket.DefaultProgOpts(), false)
	})

	t.Run("syn-port-multi-ok", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port 6666 and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := rawpacket.DefaultProgOpts()
		opts.NopInstLen = opts.MaxProgSize - 50

		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", 255, 2, opts, true)
	})

	t.Run("syn-port-multi-ko", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ko1",
				BPFFilter: "tcp dst port 6666 and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ko2",
				BPFFilter: "tcp dst port 7777 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := rawpacket.DefaultProgOpts()
		opts.NopInstLen = opts.MaxProgSize - 50

		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", probes.TCActUnspec, 2, opts, true)
	})

	t.Run("syn-port-multi-syntax-err", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ko",
				BPFFilter: "tcp dst port number and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := rawpacket.DefaultProgOpts()
		opts.NopInstLen = opts.MaxProgSize - 50

		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", 255, 1, opts, false)
	})

	t.Run("syn-port-multi-limit-ok", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ko1",
				BPFFilter: "tcp dst port number and tcp[tcpflags] == tcp-syn",
			},
			{
				RuleID:    "ko2",
				BPFFilter: "tcp dst port 7777 and tcp[tcpflags] == tcp-syn",
			},
		}

		opts := rawpacket.DefaultProgOpts()
		opts.MaxTailCalls = 0
		opts.NopInstLen = opts.MaxProgSize - 50

		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", 255, 2, opts, false)
	})

	t.Run("tcp-elf-magic-number", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp[((tcp[12] & 0xf0) >> 2):4] = 0x7f454c46",
			},
		}
		testRawPacketFilter(t, filters, "test/raw_packet_tail_calls", probes.TCActUnspec, 1, rawpacket.DefaultProgOpts(), true)
	})

	t.Run("tcp-bpfdoor-magic-number", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "udp[8:2]=0x7255 or (icmp[8:2]=0x7255 and icmp[icmptype] == icmp-echo) or tcp[((tcp[12]&0xf0)>>2):2]=0x5293 or tcp[((tcp[12]&0xf0)>>2)+26:4]=0x39393939",
			},
		}
		testRawPacketFilter(t, filters, "test/raw_packet_bpfdoor_magic_number", 255, 1, rawpacket.DefaultProgOpts(), true)
	})
}
