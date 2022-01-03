// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestBPFEventLoad(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_prog_load",
			Expression: `bpf.cmd == BPF_PROG_LOAD && process.file.name == "syscall_go_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		if _, ok := err.(ErrUnsupportedArch); ok {
			t.Skip(err)
		} else {
			t.Fatal(err)
		}
	}

	t.Run("prog_load", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "-load-bpf")
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "bpf", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(model.BpfProgTypeKprobe), event.BPF.Program.Type, "wrong program type")

			if !validateBPFSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}

func TestBPFEventMap(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_map_create",
			Expression: `bpf.cmd == BPF_MAP_CREATE && process.file.name == "syscall_go_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		if _, ok := err.(ErrUnsupportedArch); ok {
			t.Skip(err)
		} else {
			t.Fatal(err)
		}
	}

	t.Run("map_lookup", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "-load-bpf", "-clone-bpf")
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "bpf", event.GetType(), "wrong event type")

			// TODO: the manager generate a map call, we need to the name available to select the right event
			//assert.Equal(t, uint32(model.BpfMapTypeHash), event.BPF.Map.Type, "wrong map type")

			if !validateBPFSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}
