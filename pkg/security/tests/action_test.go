// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestActionKill(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "bpf_send_signal is not supported on this kernel and agent is running in container mode", func(kv *kernel.Version) bool {
		return !kv.SupportBPFSendSignal() && config.IsContainerized()
	})

	rule := &rules.RuleDefinition{
		ID: "kill_action",
		// using a wilcard to avoid approvers on basename. events will not match thus will be noisy
		Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-action"`,
		Actions: []*rules.ActionDefinition{
			{
				Kill: &rules.KillDefinition{
					Signal: "SIGUSR2",
				},
			},
		},
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	testFile, _, err := test.Path("test-kill-action")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("kill_action", func(t *testing.T) {
		sigpipeCh := make(chan os.Signal, 1)
		signal.Notify(sigpipeCh, syscall.SIGUSR2)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := runSyscallTesterFunc(
			timeoutCtx, t, syscallTester,
			"set-signal-handler", ";",
			"open", testFile, ";",
			"sleep", "2", ";",
			"wait-signal", ";",
			"signal", "sigusr2", strconv.Itoa(int(os.Getpid())),
		); err != nil {
			t.Fatal("no signal")
		}

		select {
		case <-sigpipeCh:
		case <-time.After(time.Second * 3):
			t.Error("signal timeout")
		}
	})
}
