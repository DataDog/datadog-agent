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
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestEBPFLessAttach(t *testing.T) {
	// This test doesn't support nested ptrace, so doesn't run with the wrapper
	SkipIfNotAvailable(t)

	if ebpfLessEnabled {
		t.Skip("doesn't support nested ptrace")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ebpfless_attach",
			Expression: `mkdir.file.name == "test-ebpfless-attach"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{ebpfLessEnabled: true, dontWaitEBPFLessClient: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		go func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

			cmd := exec.CommandContext(timeoutCtx, "sh", "-c", "rm -rf /tmp/test-ebpfless-attach; sleep 5; mkdir -p /tmp/test-ebpfless-attach; sleep 2")
			cmd.Start()

			defer os.Remove("/tmp/test-ebpfless-attach")

			pid := cmd.Process.Pid

			opts := ptracer.Opts{
				ProcScanDisabled: true,
			}

			err := ptracer.Attach(pid, constants.DefaultEBPFLessProbeAddr, opts)
			if err != nil {
				fmt.Printf("unable to attach: %v", err)
			}
			cancel()
		}()
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_ebpfless_attach")
	})
}
