// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestBPFEventLoad(t *testing.T) {
	checkKernelCompatibility(t, "< 4.15 kernels", func(kv *kernel.Version) bool {
		return !kv.IsRH7Kernel() && kv.Code < kernel.Kernel4_15
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_prog_load",
			Expression: `bpf.cmd == BPF_PROG_LOAD && bpf.prog.name == "kprobe_vfs_open" && process.file.name == "syscall_go_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("prog_load", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "-load-bpf")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "bpf", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(model.BpfProgTypeKprobe), event.BPF.Program.Type, "wrong program type")

			test.validateBPFSchema(t, event)
		})
	})
}

func TestBPFEventMap(t *testing.T) {
	checkKernelCompatibility(t, "< 4.15 kernels", func(kv *kernel.Version) bool {
		return !kv.IsRH7Kernel() && kv.Code < kernel.Kernel4_15
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_map_create",
			Expression: `bpf.cmd == BPF_MAP_CREATE && bpf.map.name == "cache" && process.file.name == "syscall_go_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("map_lookup", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "-load-bpf", "-clone-bpf")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "bpf", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(model.BpfMapTypeHash), event.BPF.Map.Type, "wrong map type")

			test.validateBPFSchema(t, event)
		})
	})
}

func TestBPFCwsMapConstant(t *testing.T) {
	checkKernelCompatibility(t, "< 4.15 kernels", func(kv *kernel.Version) bool {
		return !kv.IsRH7Kernel() && kv.Code < kernel.Kernel4_15
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_map_create",
			Expression: `bpf.cmd == BPF_MAP_CREATE && bpf.map.name in CWS_MAP_NAMES && process.file.name == "syscall_go_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("map_lookup", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "-load-bpf")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "bpf", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(model.BpfMapTypeArray), event.BPF.Map.Type, "wrong map type")

			test.validateBPFSchema(t, event)
		})
	})
}
