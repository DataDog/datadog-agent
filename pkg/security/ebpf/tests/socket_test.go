// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata && cgo

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

func TestCgroupSocketReturnCode(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		err := probes.CheckCgroupSocketReturnCode(loadColSpec(t).Programs)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("ko", func(t *testing.T) {
		insts := asm.Instructions{
			asm.Mov.Imm(asm.R0, 0),
			asm.Return(),
		}
		progSpecs := make(map[string]*ebpf.ProgramSpec)
		progSpecs["test"] = &ebpf.ProgramSpec{
			Name:         "test",
			Type:         ebpf.CGroupSock,
			Instructions: insts,
		}

		err := probes.CheckCgroupSocketReturnCode(progSpecs)
		if err == nil {
			t.Error("expected error")
		}
	})
}
