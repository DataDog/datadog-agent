// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestEBPFLessAttach(t *testing.T) {
	t.Skip("not stable yet")

	// This test doesn't support nested ptrace, so doesn't run with the wrapper
	SkipIfNotAvailable(t)

	if ebpfLessEnabled {
		t.Skip("doesn't support nested ptrace")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ebpfless_attach",
			Expression: `open.file.name == "test-ebpfless-attach"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{ebpfLessEnabled: true, dontWaitEBPFLessClient: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	doneCh := make(chan bool)

	test.WaitSignal(t, func() error {
		go func() {
			testFile, _, err := test.Path("test-ebpfless-attach")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGUSR1)
			defer signal.Stop(sigCh)

			timeoutCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()

			cmd := exec.CommandContext(timeoutCtx, syscallTester,
				"set-signal-handler", ";",
				"signal", "sigusr1", strconv.Itoa(int(os.Getpid())), ";",
				"wait-signal", ";",
				"open", testFile, ";",
				"sleep", "1",
			)
			cmd.Start()

			pid := cmd.Process.Pid
			opts := ptracer.Opts{
				ProcScanDisabled: true,
				Verbose:          true,
				Debug:            true,
				AttachedCb: func() {
					syscall.Kill(pid, syscall.SIGUSR2)
				},
			}

			// syscall tester to be reading to be tested
			_ = <-sigCh
			if err = ptracer.Attach([]int{pid}, constants.DefaultEBPFLessProbeAddr, opts); err != nil {
				fmt.Printf("unable to attach: %v", err)
			}
			doneCh <- true
		}()
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_ebpfless_attach")
	})

	select {
	case <-doneCh:
	case <-time.After(time.Second * 10):
		t.Error("test timeout")
	}
}
