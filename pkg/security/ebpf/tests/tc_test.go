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

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
)

func TestTCActUnspec(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		err := probes.CheckUnspecReturnCode(loadColSpec(t).Programs)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("ko", func(t *testing.T) {
		insts := asm.Instructions{
			asm.Mov.Imm(asm.R0, probes.TCActOk),
			asm.Return(),
		}
		progSpecs := make(map[string]*ebpf.ProgramSpec)
		progSpecs["test"] = &ebpf.ProgramSpec{
			Name:         "test",
			Type:         ebpf.SchedCLS,
			Instructions: insts,
		}

		err := probes.CheckUnspecReturnCode(progSpecs)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("rawpacket", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}
		progSpecs, err := rawpacket.FiltersToProgramSpecs(1, 2, filters, rawpacket.DefaultProgOpts())
		if err != nil {
			t.Error(err)
		}

		progSpecsMap := make(map[string]*ebpf.ProgramSpec)
		for _, progSpec := range progSpecs {
			progSpecsMap[progSpec.Name] = progSpec
		}
		err = probes.CheckUnspecReturnCode(progSpecsMap)
		if err != nil {
			t.Error(err)
		}
	})
}
