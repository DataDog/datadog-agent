// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestActionKill(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		checkKernelCompatibility(t, "bpf_send_signal is not supported on this kernel and agent is running in container mode", func(kv *kernel.Version) bool {
			return !kv.SupportBPFSendSignal() && env.IsContainerized()
		})
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "kill_action_usr2",
			Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-action-usr2"`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGUSR2",
					},
				},
			},
		},
		{
			ID:         "kill_action_kill",
			Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-action-kill"`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("kill-action-usr2", func(t *testing.T) {
		testFile, _, err := test.Path("test-kill-action-usr2")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		err = test.GetEventSent(t, func() error {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGUSR1)
			defer signal.Stop(sigCh)

			timeoutCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()

			if err := runSyscallTesterFunc(
				timeoutCtx, t, syscallTester,
				"set-signal-handler", ";",
				"open", testFile, ";",
				"sleep", "1", ";",
				"open", testFile, ";",
				"wait-signal", ";",
				"signal", "sigusr1", strconv.Itoa(int(os.Getpid())), ";",
				"sleep", "1",
			); err != nil {
				t.Error(err)
			}

			select {
			case <-sigCh:
			case <-time.After(time.Second * 3):
				t.Error("signal timeout")
			}
			return nil
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*3, "kill_action_usr2")
		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("kill_action_usr2")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(testMod *testModule, obj interface{}) {
				if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal="sigusr2")]`); err != nil {
					t.Error(err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

	t.Run("kill-action-kill", func(t *testing.T) {
		testFile, _, err := test.Path("test-kill-action-kill")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		err = test.GetEventSent(t, func() error {
			ch := make(chan bool, 1)

			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				cmd := exec.CommandContext(timeoutCtx, syscallTester, "open", testFile, ";", "sleep", "1", ";", "open", testFile, ";", "sleep", "5")
				_ = cmd.Run()

				ch <- true
			}()

			select {
			case <-ch:
			case <-time.After(time.Second * 3):
				t.Error("signal timeout")
			}
			return nil
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*5, "kill_action_kill")

		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("kill_action_kill")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(testMod *testModule, obj interface{}) {
				if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal="sigkill")]`); err != nil {
					t.Error(err)
				}
				if _, err = jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.exited_at=~/20.*/)]`); err != nil {
					t.Error(err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
}

func TestActionKillExcludeBinary(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "bpf_send_signal is not supported on this kernel and agent is running in container mode", func(kv *kernel.Version) bool {
		return !kv.SupportBPFSendSignal() && env.IsContainerized()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "kill_action_kill_exclude",
			Expression: `exec.file.name == "sleep" && exec.argv in ["1234567"]`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		},
	}

	executable := which(t, "sleep")

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{enforcementExcludeBinary: executable}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	killed := atomic.NewBool(false)

	err = test.GetEventSent(t, func() error {
		go func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd := exec.CommandContext(timeoutCtx, "sleep", "1234567")
			_ = cmd.Run()

			killed.Store(true)
		}()

		return nil
	}, func(rule *rules.Rule, event *model.Event) bool {
		return true
	}, time.Second*5, "kill_action_kill_exclude")

	if err != nil {
		t.Error("should get an event")
	}

	if killed.Load() {
		t.Error("shouldn't be killed")
	}
}

func TestActionKillRuleSpecific(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		checkKernelCompatibility(t, "bpf_send_signal is not supported on this kernel and agent is running in container mode", func(kv *kernel.Version) bool {
			return !kv.SupportBPFSendSignal() && env.IsContainerized()
		})
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "kill_action_kill",
			Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-action-kill"`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		},
		{
			ID:         "kill_action_no_kill",
			Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-action-kill"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	testFile, _, err := test.Path("test-kill-action-kill")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	err = test.GetEventSent(t, func() error {
		ch := make(chan bool, 1)

		go func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cmd := exec.CommandContext(timeoutCtx, syscallTester, "open", testFile, ";", "sleep", "1", ";", "open", testFile, ";", "sleep", "5")
			_ = cmd.Run()

			ch <- true
		}()

		select {
		case <-ch:
		case <-time.After(time.Second * 3):
			t.Error("signal timeout")
		}
		return nil
	}, func(rule *rules.Rule, event *model.Event) bool {
		return true
	}, time.Second*5, "kill_action_kill")

	if err != nil {
		t.Error(err)
	}

	err = retry.Do(func() error {
		msg := test.msgSender.getMsg("kill_action_kill")
		if msg == nil {
			return errors.New("not found")
		}
		validateMessageSchema(t, string(msg.Data))

		jsonPathValidation(test, msg.Data, func(testMod *testModule, obj interface{}) {
			if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal="sigkill")]`); err != nil {
				t.Error(err)
			}
			if _, err = jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.exited_at=~/20.*/)]`); err != nil {
				t.Error(err)
			}
		})

		return nil
	}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)

	err = retry.Do(func() error {
		msg := test.msgSender.getMsg("kill_action_no_kill")
		if msg == nil {
			return errors.New("not found")
		}
		validateMessageSchema(t, string(msg.Data))

		jsonPathValidation(test, msg.Data, func(testMod *testModule, obj interface{}) {
			if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions`); err == nil {
				t.Error(errors.New("unexpected rule action"))
			}
		})

		return nil
	}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)
}

func TestActionKillDisarm(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "bpf_send_signal is not supported on this kernel and agent is running in container mode", func(kv *kernel.Version) bool {
		return !kv.SupportBPFSendSignal() && env.IsContainerized()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "kill_action_disarm_executable",
			Expression: `exec.envs in ["TARGETTOKILL"] && container.id == ""`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		},
		{
			ID:         "kill_action_disarm_container",
			Expression: `exec.envs in ["TARGETTOKILL"] && container.id != ""`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		},
	}

	sleep := which(t, "sleep")
	const (
		enforcementDisarmerContainerPeriod  = 10 * time.Second
		enforcementDisarmerExecutablePeriod = 10 * time.Second
	)

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		enforcementDisarmerContainerEnabled:     true,
		enforcementDisarmerContainerMaxAllowed:  1,
		enforcementDisarmerContainerPeriod:      enforcementDisarmerContainerPeriod,
		enforcementDisarmerExecutableEnabled:    true,
		enforcementDisarmerExecutableMaxAllowed: 1,
		enforcementDisarmerExecutablePeriod:     enforcementDisarmerExecutablePeriod,
		eventServerRetention:                    1 * time.Nanosecond,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	testKillActionSuccess := func(t *testing.T, ruleID string, cmdFunc func(context.Context)) {
		test.msgSender.flush()
		err := test.GetEventSent(t, func() error {
			ch := make(chan bool, 1)

			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				cmdFunc(timeoutCtx)

				ch <- true
			}()

			select {
			case <-ch:
			case <-time.After(time.Second * 3):
				t.Error("signal timeout")
			}
			return nil
		}, func(_ *rules.Rule, _ *model.Event) bool {
			return true
		}, time.Second*5, ruleID)
		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg(ruleID)
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal="sigkill")]`); err != nil {
					t.Error(err)
				}
				if _, err = jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.exited_at=~/20.*/)]`); err != nil {
					t.Error(err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	}

	testKillActionIgnored := func(t *testing.T, ruleID string, cmdFunc func(context.Context)) {
		test.msgSender.flush()
		err := test.GetEventSent(t, func() error {
			cmdFunc(nil)
			return nil
		}, func(_ *rules.Rule, _ *model.Event) bool {
			return true
		}, time.Second*5, ruleID)
		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg(ruleID)
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions`); err == nil {
					t.Error(errors.New("unexpected rule action"))
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	}

	t.Run("executable", func(t *testing.T) {
		// test that we can kill processes with the same executable more than once
		for i := 0; i < 2; i++ {
			t.Logf("test iteration %d", i)
			testKillActionSuccess(t, "kill_action_disarm_executable", func(ctx context.Context) {
				cmd := exec.CommandContext(ctx, syscallTester, "sleep", "5")
				cmd.Env = []string{"TARGETTOKILL=1"}
				_ = cmd.Run()
			})
		}

		// test that another executable dismars the kill action
		testKillActionIgnored(t, "kill_action_disarm_executable", func(_ context.Context) {
			cmd := exec.Command(sleep, "1")
			cmd.Env = []string{"TARGETTOKILL=1"}
			_ = cmd.Run()
		})

		// test that the kill action is re-armed after both executable cache entries have expired
		// sleep for: (TTL + cache flush period + 1s) to ensure the cache is flushed
		time.Sleep(enforcementDisarmerExecutablePeriod + 5*time.Second + 1*time.Second)
		testKillActionSuccess(t, "kill_action_disarm_executable", func(_ context.Context) {
			cmd := exec.Command(sleep, "1")
			cmd.Env = []string{"TARGETTOKILL=1"}
			_ = cmd.Run()
		})
	})

	t.Run("container", func(t *testing.T) {
		dockerInstance, err := test.StartADocker()
		if err != nil {
			t.Fatalf("failed to start a Docker instance: %v", err)
		}
		defer dockerInstance.stop()

		// test that we can kill processes within the same container more than once
		for i := 0; i < 2; i++ {
			t.Logf("test iteration %d", i)
			testKillActionSuccess(t, "kill_action_disarm_container", func(_ context.Context) {
				cmd := dockerInstance.Command("env", []string{"-i", "-", "TARGETTOKILL=1", "sleep", "5"}, []string{})
				_ = cmd.Run()
			})
		}

		newDockerInstance, err := test.StartADocker()
		if err != nil {
			t.Fatalf("failed to start a second Docker instance: %v", err)
		}
		defer newDockerInstance.stop()

		// test that another container dismars the kill action
		testKillActionIgnored(t, "kill_action_disarm_container", func(_ context.Context) {
			cmd := newDockerInstance.Command("env", []string{"-i", "-", "TARGETTOKILL=1", "sleep", "1"}, []string{})
			_ = cmd.Run()
		})

		// test that the kill action is re-armed after both container cache entries have expired
		// sleep for: (TTL + cache flush period + 1s) to ensure the cache is flushed
		time.Sleep(enforcementDisarmerContainerPeriod + 5*time.Second + 1*time.Second)
		testKillActionSuccess(t, "kill_action_disarm_container", func(_ context.Context) {
			cmd := newDockerInstance.Command("env", []string{"-i", "-", "TARGETTOKILL=1", "sleep", "5"}, []string{})
			_ = cmd.Run()
		})
	})
}
