// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

package tests

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestReplay(t *testing.T) {
	SkipIfNotAvailable(t)

	t.Run("host-event", func(t *testing.T) {
		ruleDefs := []*rules.RuleDefinition{
			{
				ID:         "test_rule_replay_host",
				Expression: `exec.comm in ["testsuite"]`,
			},
		}

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay_host")
				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)

				// validate that pid 1 is reported as an exec
				ancestor := e.ProcessContext.Ancestor
				for ancestor != nil {
					if ancestor.Pid == 1 && !ancestor.IsExec {
						t.Errorf("pid1 should be reported as an Exec: %+v", e)
					}
					ancestor = ancestor.Ancestor
				}

				gotEvent.Store(true)
			},
		}))

		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from snapshot")
	})

	t.Run("container-event", func(t *testing.T) {
		ruleDefs := []*rules.RuleDefinition{
			{
				ID:         "test_rule_replay_container",
				Expression: `exec.comm in ["sleep"] && process.argv in ["123"] && process.container.id != ""`,
			},
		}

		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
			return kv.IsSuse12Kernel()
		})

		dockerWrapper, err := newDockerCmdWrapper("/tmp", "/tmp", "ubuntu", "")
		if err != nil {
			t.Fatalf("failed to create docker wrapper: %v", err)
		}

		if _, err := dockerWrapper.start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			output, err := dockerWrapper.stop()
			if err != nil {
				t.Errorf("failed to stop docker wrapper: %v\n%s", err, string(output))
			}
		})

		sleepCtx, cancel := context.WithCancel(context.Background())

		go func() {
			cmd := dockerWrapper.CommandContext(sleepCtx, "sh", []string{"-c", "sleep 123"}, nil)
			_ = cmd.Run()
		}()

		// wait a bit so that the command is running and captured by the snapshot
		time.Sleep(2 * time.Second)

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay_container")
				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)
				gotEvent.Store(true)
			},
		}))

		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		// make sure the cancel happens before the test module is closed
		defer cancel()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from snapshot")
	})

	t.Run("replay-event", func(t *testing.T) {
		ruleDefs := []*rules.RuleDefinition{
			{
				ID:         "test_rule_replay",
				Expression: `event.source == "replay" && exec.comm in ["sleep"]`,
			},
		}

		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
			return kv.IsSuse12Kernel()
		})

		dockerWrapper, err := newDockerCmdWrapper("/tmp", "/tmp", "ubuntu", "")
		if err != nil {
			t.Fatalf("failed to create docker wrapper: %v", err)
		}

		if _, err := dockerWrapper.start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			output, err := dockerWrapper.stop()
			if err != nil {
				t.Errorf("failed to stop docker wrapper: %v\n%s", err, string(output))
			}
		})

		var cmd *exec.Cmd
		go func() {
			cmd = dockerWrapper.Command("sh", []string{"-c", "sleep 123"}, nil)
			_ = cmd.Run()
		}()

		// wait a bit so that the command is running and captured by the snapshot
		time.Sleep(2 * time.Second)

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay")
				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)
				gotEvent.Store(true)
			},
		}))

		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()
		defer cmd.Cancel()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from replay")
	})
}
