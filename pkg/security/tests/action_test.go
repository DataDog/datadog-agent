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
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestActionKill(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
			return env.IsContainerized()
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
				"open", syscallTester, ";",
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
		}, func(_ *rules.Rule, _ *model.Event) bool {
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

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal == 'SIGUSR2')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.status == 'performed')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
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

				cmd := exec.CommandContext(timeoutCtx, syscallTester, "open", testFile, ";", "sleep", "1", ";", "open", syscallTester, ";", "sleep", "5")
				_ = cmd.Run()

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

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal == 'SIGKILL')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.exited_at =~ /20.*/)]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.status == 'performed')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
}

func TestActionKillExcludeBinary(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
		return env.IsContainerized()
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

	sleepCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	killed := atomic.NewBool(false)

	err = test.GetEventSent(t, func() error {
		go func() {

			cmd := exec.CommandContext(sleepCtx, "sleep", "1234567")
			_ = cmd.Run()

			killed.Store(true)
		}()

		return nil
	}, func(_ *rules.Rule, _ *model.Event) bool {
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
		checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
			return env.IsContainerized()
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

			cmd := exec.CommandContext(timeoutCtx, syscallTester, "open", testFile, ";", "sleep", "1", ";", "open", syscallTester, ";", "sleep", "5")
			_ = cmd.Run()

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

		jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
			if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal == 'SIGKILL')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
				t.Errorf("element not found %s => %v", string(msg.Data), err)
			}
			if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.exited_at =~ /20.*/)]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
				t.Errorf("element not found %s => %v", string(msg.Data), err)
			}
			if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.status == 'performed')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
				t.Errorf("element not found %s => %v", string(msg.Data), err)
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

		jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
			if _, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions`); err == nil {
				t.Errorf("unexpected rule action %s", string(msg.Data))
			}
		})

		return nil
	}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)
}

func testActionKillDisarm(t *testing.T, test *testModule, sleep, syscallTester string, disarmerPeriod time.Duration) {
	t.Helper()

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
			case <-time.After(time.Second * 8):
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
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal == 'SIGKILL')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.exited_at =~ /20.*/)]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.status == 'performed')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	}

	testKillActionDisarmed := func(t *testing.T, ruleID string, cmdFunc func(context.Context)) {
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
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.signal == 'SIGKILL')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.status == 'rule_disarmed')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
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

		// test that another executable disarms the kill action
		testKillActionDisarmed(t, "kill_action_disarm_executable", func(_ context.Context) {
			cmd := exec.Command(sleep, "1")
			cmd.Env = []string{"TARGETTOKILL=1"}
			_ = cmd.Run()
		})

		// test that the kill action is re-armed after both executable cache entries have expired
		// sleep for: (TTL + cache flush period + 1s) to ensure the cache is flushed
		time.Sleep(disarmerPeriod + 5*time.Second + 1*time.Second)
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

		// test that another container disarms the kill action
		testKillActionDisarmed(t, "kill_action_disarm_container", func(_ context.Context) {
			cmd := newDockerInstance.Command("env", []string{"-i", "-", "TARGETTOKILL=1", "sleep", "1"}, []string{})
			_ = cmd.Run()
		})

		// test that the kill action is re-armed after both container cache entries have expired
		// sleep for: (TTL + cache flush period + 1s) to ensure the cache is flushed
		time.Sleep(disarmerPeriod + 5*time.Second + 1*time.Second)
		testKillActionSuccess(t, "kill_action_disarm_container", func(_ context.Context) {
			cmd := newDockerInstance.Command("env", []string{"-i", "-", "TARGETTOKILL=1", "sleep", "5"}, []string{})
			_ = cmd.Run()
		})
	})
}

func TestActionKillDisarm(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
		return env.IsContainerized()
	})

	sleep := which(t, "sleep")

	const (
		enforcementDisarmerPeriod = 4 * time.Second
	)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "kill_action_disarm_executable",
			Expression: `exec.envs in ["TARGETTOKILL"] && process.container.id == ""`,
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
			Expression: `exec.envs in ["TARGETTOKILL"] && process.container.id != ""`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
					},
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		enforcementDisarmerContainerEnabled:     true,
		enforcementDisarmerContainerMaxAllowed:  1,
		enforcementDisarmerContainerPeriod:      enforcementDisarmerPeriod,
		enforcementDisarmerExecutableEnabled:    true,
		enforcementDisarmerExecutableMaxAllowed: 1,
		enforcementDisarmerExecutablePeriod:     enforcementDisarmerPeriod,
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

	testActionKillDisarm(t, test, sleep, syscallTester, enforcementDisarmerPeriod)
}

func TestActionHash(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("skipping in docker, not  sharing the same pid ns and doesn't have a container ID")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "hash_action_open",
			Expression: `open.file.path == "{{.Root}}/test-hash-action" && open.flags&O_CREAT == O_CREAT`,
			Actions: []*rules.ActionDefinition{
				{
					Hash: &rules.HashDefinition{},
				},
			},
		},
		{
			ID:         "hash_action_exec",
			Expression: `exec.file.path == "{{.Root}}/test-hash-action-exec_tester"`,
			Actions: []*rules.ActionDefinition{
				{
					Hash: &rules.HashDefinition{},
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-hash-action")
	if err != nil {
		t.Fatal(err)
	}

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	testExecutable, _, err := test.Path("test-hash-action-exec_tester")
	if err != nil {
		t.Fatal(err)
	}

	if err = copyFile(syscallTester, testExecutable, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testExecutable)

	done := make(chan bool, 10)

	t.Run("open-process-exit", func(t *testing.T) {
		test.msgSender.flush()
		test.WaitSignalFromRule(t, func() error {
			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := runSyscallTesterFunc(
					timeoutCtx, t, syscallTester,
					"slow-write", "2", testFile, "aaa",
				); err != nil {
					t.Error(err)
				}

				done <- true
			}()
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "hash_action_open")
		}, "hash_action_open")

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("hash_action_open")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.state == 'Done')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.trigger == 'process_exit')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.file.hashes`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(500*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)

		<-done
	})

	t.Run("open-timeout", func(t *testing.T) {
		test.msgSender.flush()
		test.WaitSignalFromRule(t, func() error {
			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if err := runSyscallTesterFunc(
					timeoutCtx, t, syscallTester,
					// exceed the file hasher timeout, use fork to force an event that will trigger the flush mechanism
					"slow-write", "2", testFile, "aaa", ";", "sleep", "4", ";", "fork", ";", "sleep", "1",
				); err != nil {
					t.Error(err)
				}

				done <- true
			}()
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "hash_action_open")
		}, "hash_action_open")

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("hash_action_open")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.state == 'Done')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.trigger == 'timeout')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.file.hashes`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(500*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)

		<-done
	})

	t.Run("exec", func(t *testing.T) {
		test.msgSender.flush()
		test.WaitSignalFromRule(t, func() error {
			cmd := exec.Command(testExecutable, "sleep", "1")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("output: %s", string(out))
			}
			return err
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "hash_action_exec")
		}, "hash_action_exec")
		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("hash_action_exec")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.state == 'Done')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.trigger == 'process_exit')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.file.hashes`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(500*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

	t.Run("open-force-path", func(t *testing.T) {
		// test that we correctly force a path resolution when we run the hash action
		newRuleDefs := []*rules.RuleDefinition{
			{
				ID:         "hash_action_open_no_path",
				Expression: `open.flags&O_CREAT == O_CREAT && process.file.name == "syscall_tester"`,
				Actions: []*rules.ActionDefinition{
					{
						Hash: &rules.HashDefinition{
							Field: "open.file",
						},
					},
				},
			},
		}

		// Set the new policy and reload (without closing/restarting the module)
		// On reload, exec events are replayed for running processes, so the kill rule should trigger
		if err := setTestPolicy(commonCfgDir, nil, newRuleDefs); err != nil {
			t.Fatalf("failed to set new policy: %v", err)
		}

		err := test.reloadPolicies()
		if err != nil {
			t.Fatalf("failed to reload policies: %v", err)
		}

		test.msgSender.flush()
		test.WaitSignalFromRule(t, func() error {
			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := runSyscallTesterFunc(
					timeoutCtx, t, syscallTester,
					"slow-write", "2", testFile, "aaa",
				); err != nil {
					t.Error(err)
				}

				done <- true
			}()
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "hash_action_open_no_path")
		}, "hash_action_open_no_path")

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("hash_action_open_no_path")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.state == 'Done')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_actions[?(@.trigger == 'process_exit')]`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
				if el, err := jsonpath.JsonPathLookup(obj, `$.file.hashes`); err != nil || el == nil || len(el.([]interface{})) == 0 {
					t.Errorf("element not found %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(500*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)

		<-done
	})
}

func TestActionKillWithSignature(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
			return env.IsContainerized()
		})
	}

	// Create a temporary file that will be used in tail arguments
	testFile, err := os.CreateTemp("", "test-kill-signature-*")
	if err != nil {
		t.Fatal(err)
	}
	testFilePath := testFile.Name()
	testFile.Close()
	defer os.Remove(testFilePath)

	// Rule to trigger on exec of tail with the test file as argument
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_exec_trigger",
			Expression: `exec.file.name == "tail" && exec.argv in ["` + testFilePath + `"]`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var capturedSignature string
	var tailCmd *exec.Cmd

	// Cleanup function to kill any remaining tail processes for this test file
	cleanupTail := func() {
		exec.Command("pkill", "-f", "tail -F "+testFilePath).Run()
	}
	defer cleanupTail()

	// Start tail -F and wait for the rule to trigger
	test.WaitSignalFromRule(t, func() error {
		// Start tail
		tailCmd = exec.Command("tail", "-F", testFilePath)
		if err := tailCmd.Start(); err != nil {
			return err
		}
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_exec_trigger")
		// Capture the signature from the event
		capturedSignature = event.FieldHandlers.ResolveSignature(event)
	}, "test_exec_trigger")

	// Verify we got a valid signature
	if capturedSignature == "" {
		t.Fatal("captured signature is empty")
	}

	// Verify that tail is still running
	if tailCmd.ProcessState != nil && tailCmd.ProcessState.Exited() {
		t.Fatal("tail process should still be running after first rule trigger")
	}

	// Create a new rule with kill action that matches the captured signature
	firstTailPid := strconv.Itoa(tailCmd.Process.Pid)
	newRuleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_exec_trigger",
			Expression: `exec.file.name == "tail" && exec.argv in ["` + testFilePath + `"] && process.pid != ` + firstTailPid,
		},
		{
			ID:         "test_kill_with_signature",
			Expression: `exec.file.name == "tail" && exec.argv in ["` + testFilePath + `"] && event.signature == "` + capturedSignature + `" && process.pid == ` + firstTailPid,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal:                    "SIGKILL",
						Scope:                     "process",
						DisableContainerDisarmer:  true,
						DisableExecutableDisarmer: true,
					},
				},
			},
		},
	}

	// Set the new policy and reload (without closing/restarting the module)
	// On reload, exec events are replayed for running processes, so the kill rule should trigger
	if err := setTestPolicy(commonCfgDir, nil, newRuleDefs); err != nil {
		t.Fatalf("failed to set new policy: %v", err)
	}

	// Reload the policy and wait for the kill rule to trigger
	// Use GetEventSent instead of WaitSignal because ActionReports are filled in HandleActions
	// which is called AFTER RuleMatch (used by WaitSignal) but BEFORE SendEvent (used by GetEventSent)
	err = test.GetEventSent(t, func() error {
		err := test.reloadPolicies()
		if err != nil {
			return fmt.Errorf("failed to reload policies: %w", err)
		}
		// Trigger a small event to force the replay of cached events.
		// The replay only happens in handleEvent when a new eBPF event arrives.
		exec.Command("true").Run()
		return nil
	}, func(rule *rules.Rule, event *model.Event) bool {
		assertTriggeredRule(t, rule, "test_kill_with_signature")

		// Verify the kill action was performed using the event's action reports
		assert.Equal(t, 1, len(event.ActionReports), "expected one action report")
		if len(event.ActionReports) == 1 {
			report := event.ActionReports[0]
			if killReport, ok := report.(*sprobe.KillActionReport); ok {
				assert.Equal(t, "SIGKILL", killReport.Signal, "unexpected signal")
				assert.Equal(t, "process", killReport.Scope, "unexpected scope")
				assert.Equal(t, sprobe.KillActionStatusPerformed, killReport.Status, "unexpected status")
			}
		}
		return true
	}, 10*time.Second, "test_kill_with_signature")
	if err != nil {
		t.Fatal(err)
	}

	// Verify that tail was killed
	done := make(chan error, 1)
	go func() {
		done <- tailCmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("tail process should have been killed but is still running")
	}

	// Now start a new tail process - it should NOT be killed because it has a different signature
	var tailCmd2 *exec.Cmd
	test.WaitSignalFromRule(t, func() error {
		tailCmd2 = exec.Command("tail", "-f", testFilePath)
		return tailCmd2.Start()
	}, func(_ *model.Event, rule *rules.Rule) {
		// Only test_exec_trigger should match because the signature is different
		assertTriggeredRule(t, rule, "test_exec_trigger")
	}, "test_exec_trigger")

	// Verify that the second tail is still running (not killed due to different signature)
	done2 := make(chan error, 1)
	go func() {
		done2 <- tailCmd2.Wait()
	}()
	select {
	case <-done2:
		t.Fatal("second tail process should still be running (different signature)")
	case <-time.After(3 * time.Second):
		// Process is still running as expected
	}

	// Cleanup second tail
	tailCmd2.Process.Kill()
	<-done2 // Wait for the goroutine to finish instead of calling Wait() again
}

func TestActionKillContainerWithSignature(t *testing.T) {
	SkipIfNotAvailable(t)
	flake.MarkOnJobName(t, "cws_host")

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "skip on CentOS7", func(kv *kernel.Version) bool {
		return kv.IsRH7Kernel()
	})

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
		return env.IsContainerized()
	})

	// 1. Start a Docker container first
	dockerInstance, err := newDockerCmdWrapper("/tmp", "/tmp", "alpine", "")
	if err != nil {
		t.Fatalf("failed to create docker wrapper: %v", err)
	}
	if _, err := dockerInstance.start(); err != nil {
		t.Fatalf("failed to start docker: %v", err)
	}
	containerKilled := false
	defer func() {
		if !containerKilled {
			dockerInstance.stop()
		}
	}()

	// 2. Create a test file inside the container at a known path
	testFilePath := "/tmp/test-container-kill-" + utils.RandString(8)
	cmd := dockerInstance.Command("touch", []string{testFilePath}, []string{})
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create test file in container: %v", err)
	}

	// 3. Initialize the test module with the rule pointing to the correct path
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_exec_trigger",
			Expression: `exec.file.name == "tail" && exec.argv in ["` + testFilePath + `"] && process.container.id != ""`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var capturedSignature string
	var tailCmd *exec.Cmd

	// Run tail inside the container and wait for the rule to trigger
	test.WaitSignalFromRule(t, func() error {
		// Start tail -f on the test file inside the container (runs indefinitely)
		tailCmd = dockerInstance.Command("tail", []string{"-f", testFilePath}, []string{})
		return tailCmd.Start()
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_container_exec_trigger")
		// Capture the signature from the event
		capturedSignature = event.FieldHandlers.ResolveSignature(event)
	}, "test_container_exec_trigger")

	// Verify we got a valid signature
	if capturedSignature == "" {
		t.Fatal("captured signature is empty")
	}

	// Create a new rule with kill action (scope container) that matches the captured signature
	newRuleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_exec_trigger",
			Expression: `exec.file.name == "tail" && exec.argv in ["` + testFilePath + `"] && process.container.id != "" && event.signature != "` + capturedSignature + `"`,
		},
		{
			ID:         "test_kill_container_with_signature",
			Expression: `exec.file.name == "tail" && exec.argv in ["` + testFilePath + `"] && process.container.id != "" && event.signature == "` + capturedSignature + `"`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						// Scope cgroup is the scope that will be sent from the BE for remediation actions
						Scope:                     "cgroup",
						DisableContainerDisarmer:  true,
						DisableExecutableDisarmer: true,
					},
				},
			},
		},
	}

	// Set the new policy and reload
	if err := setTestPolicy(commonCfgDir, nil, newRuleDefs); err != nil {
		t.Fatalf("failed to set new policy: %v", err)
	}

	// Reload the policy and wait for the kill rule to trigger
	err = test.GetEventSent(t, func() error {
		err := test.reloadPolicies()
		if err != nil {
			return fmt.Errorf("failed to reload policies: %w", err)
		}
		// sleep to let the replay events trigger the rule
		time.Sleep(time.Second)
		return nil
	}, func(rule *rules.Rule, event *model.Event) bool {
		assertTriggeredRule(t, rule, "test_kill_container_with_signature")

		// Verify the kill action was performed with container scope
		assert.Equal(t, 1, len(event.ActionReports), "expected at action report")
		if len(event.ActionReports) == 1 {
			report := event.ActionReports[0]
			if killReport, ok := report.(*sprobe.KillActionReport); ok {
				assert.Equal(t, "SIGKILL", killReport.Signal, "unexpected signal")
				assert.Equal(t, "cgroup", killReport.Scope, "unexpected scope")
				// we might get "partially kill" status if like we start by killing the container entrypoint and we're not able to kill the tail because it's already stopped
				assert.Contains(t, []sprobe.KillActionStatus{sprobe.KillActionStatusPerformed, sprobe.KillActionStatusPartiallyPerformed}, killReport.Status, "unexpected status")
			}
		}
		return true
	}, 10*time.Second, "test_kill_container_with_signature")
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the container was killed by checking if it's still running
	err = retry.Do(func() error {
		cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", dockerInstance.containerID)
		output, err := cmd.Output()
		if err != nil {
			// Container might not exist anymore, which is also fine
			return nil
		}
		if strings.TrimSpace(string(output)) == "true" {
			return errors.New("container still running")
		}
		return nil
	}, retry.Delay(200*time.Millisecond), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	if err != nil {
		t.Fatal("container should have been killed but is still running")
	}
	containerKilled = true

	// Wait for the tail command to finish to avoid zombie process
	if tailCmd != nil && tailCmd.Process != nil {
		tailCmd.Process.Kill()
		tailCmd.Wait()
	}

	// Now start a new container and run tail - it should NOT be killed because it has a different signature
	dockerInstance2, err := newDockerCmdWrapper("/tmp", "/tmp", "alpine", "")
	if err != nil {
		t.Fatalf("failed to create second docker wrapper: %v", err)
	}
	if _, err := dockerInstance2.start(); err != nil {
		t.Fatalf("failed to start second docker: %v", err)
	}
	defer dockerInstance2.stop()

	// Create the same test file in the new container
	cmd = dockerInstance2.Command("touch", []string{testFilePath}, []string{})
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create test file in second container: %v", err)
	}

	var tailCmd2 *exec.Cmd
	test.WaitSignalFromRule(t, func() error {
		tailCmd2 = dockerInstance2.Command("tail", []string{"-f", testFilePath}, []string{})
		return tailCmd2.Start()
	}, func(_ *model.Event, rule *rules.Rule) {
		// Only test_container_exec_trigger should match because the signature is different
		assertTriggeredRule(t, rule, "test_container_exec_trigger")
	}, "test_container_exec_trigger")

	// Verify that the second container is still running (not killed due to different signature)
	err = retry.Do(func() error {
		cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", dockerInstance2.containerID)
		output, err := cmd.Output()
		if err != nil {
			return errors.New("failed to inspect container")
		}
		if strings.TrimSpace(string(output)) != "true" {
			return errors.New("container not running")
		}
		return nil
	}, retry.Delay(200*time.Millisecond), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	if err != nil {
		t.Fatal("second container should still be running (different signature)")
	}

	// Kill the second tail process and wait to avoid zombie
	if tailCmd2 != nil && tailCmd2.Process != nil {
		tailCmd2.Process.Kill()
		tailCmd2.Wait()
	}
}

func TestActionKillContainerWithSignatureBroadRule(t *testing.T) {
	SkipIfNotAvailable(t)
	flake.MarkOnJobName(t, "cws_host")

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
		return env.IsContainerized()
	})

	// 1. Start a Docker container first
	dockerInstance, err := newDockerCmdWrapper("/tmp", "/tmp", "alpine", "")
	if err != nil {
		t.Fatalf("failed to create docker wrapper: %v", err)
	}
	if _, err := dockerInstance.start(); err != nil {
		t.Fatalf("failed to start docker: %v", err)
	}
	containerKilled := false
	defer func() {
		if !containerKilled {
			dockerInstance.stop()
		}
	}()

	// 2. Create a test file inside the container at a known path
	testFilePath := "/tmp/test-container-kill-broad-" + utils.RandString(8)
	cmd := dockerInstance.Command("touch", []string{testFilePath}, []string{})
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create test file in container: %v", err)
	}

	// 3. Initialize the test module with the rule pointing to the correct path
	// Use cat with the test file to uniquely identify our process
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_exec_trigger",
			Expression: `exec.file.name == "cat" && exec.argv in ["` + testFilePath + `"] && process.container.id != ""`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var capturedSignature string
	var catCmd *exec.Cmd

	// Run cat inside the container and wait for the rule to trigger
	// cat will exit immediately after reading the file, but that's fine for capturing the signature
	test.WaitSignalFromRule(t, func() error {
		catCmd = dockerInstance.Command("cat", []string{testFilePath}, []string{})
		return catCmd.Start()
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_container_exec_trigger")
		// Capture the signature from the event
		capturedSignature = event.FieldHandlers.ResolveSignature(event)
	}, "test_container_exec_trigger")

	// Wait for cat to finish
	if catCmd != nil && catCmd.Process != nil {
		catCmd.Wait()
	}

	// Verify we got a valid signature
	if capturedSignature == "" {
		t.Fatal("captured signature is empty")
	}

	// Create a new rule with kill action (scope container) using a BROAD rule
	// This rule will match ANY exec in the container with the captured signature
	newRuleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_kill_container_broad_signature",
			Expression: `exec.file.name != "" && process.container.id != "" && event.signature == "` + capturedSignature + `"`,
			Actions: []*rules.ActionDefinition{
				{
					Kill: &rules.KillDefinition{
						Signal:                    "SIGKILL",
						Scope:                     "container",
						DisableContainerDisarmer:  true,
						DisableExecutableDisarmer: true,
					},
				},
			},
		},
	}

	// Set the new policy and reload
	if err := setTestPolicy(commonCfgDir, nil, newRuleDefs); err != nil {
		t.Fatalf("failed to set new policy: %v", err)
	}

	// Reload the policy and wait for the kill rule to trigger
	// The broad rule should match the replayed exec event for the container's entrypoint (sleep)
	err = test.GetEventSent(t, func() error {
		err := test.reloadPolicies()
		if err != nil {
			return fmt.Errorf("failed to reload policies: %w", err)
		}
		// Trigger a small event to force the replay of cached events.
		// The replay only happens in handleEvent when a new eBPF event arrives.
		exec.Command("true").Run()
		return nil
	}, func(rule *rules.Rule, event *model.Event) bool {
		assertTriggeredRule(t, rule, "test_kill_container_broad_signature")

		// Verify the kill action was performed with container scope
		assert.Equal(t, 1, len(event.ActionReports), "expected at least one action report")
		if len(event.ActionReports) == 1 {
			report := event.ActionReports[0]
			if killReport, ok := report.(*sprobe.KillActionReport); ok {
				assert.Equal(t, "SIGKILL", killReport.Signal, "unexpected signal")
				assert.Equal(t, "container", killReport.Scope, "unexpected scope")
				assert.Equal(t, sprobe.KillActionStatusPerformed, killReport.Status, "unexpected status")
			}
		}
		return true
	}, 10*time.Second, "test_kill_container_broad_signature")
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the container was killed by checking if it's still running
	err = retry.Do(func() error {
		cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", dockerInstance.containerID)
		output, err := cmd.Output()
		if err != nil {
			// Container might not exist anymore, which is also fine
			return nil
		}
		if strings.TrimSpace(string(output)) == "true" {
			return errors.New("container still running")
		}
		return nil
	}, retry.Delay(200*time.Millisecond), retry.Attempts(5), retry.DelayType(retry.FixedDelay))
	if err != nil {
		t.Fatal("container should have been killed but is still running")
	}
	containerKilled = true
}

func TestRemediationCustomEvents(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		checkKernelCompatibility(t, "agent is running in container mode", func(_ *kernel.Version) bool {
			return env.IsContainerized()
		})
	}

	checkKernelCompatibility(t, "network feature", isRawPacketNotSupported)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID: "kill_remediation",
			Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-remediation" 
			&& ${process.kill_remediation_performed} != "done"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "process",
						Name:  "kill_remediation_performed",
						Value: "done",
					},
				}, {
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "process",
					},
				},
			},
			Tags: map[string]string{
				"remediation_rule": "true",
				"agent_event_id":   "AZoIdt0EAAAbKF9Rg_3TKKJ",
				"creator_uuid":     "b6497050-10b2-11f0-a294-324b18620407",
				"creator_name":     "Allan Turing",
				"creator_handle":   "allan.turing@example.com",
			},
		},
		{
			ID: "kill_no_tags",
			Expression: `process.file.name == "syscall_tester" && open.file.path == "{{.Root}}/test-kill-no-tags" 
			&& ${process.kill_no_tags_performed} != "done"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "process",
						Name:  "kill_no_tags_performed",
						Value: "done",
					},
				}, {
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "process",
					},
				},
			},
		},
		{
			ID:         "network_remediation",
			Expression: `exec.file.name == "sleep" && exec.args in ["123"] && ${process.network_remediation_performed} != "done"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "process",
						Name:  "network_remediation_performed",
						Value: "done",
					},
				},
				{
					NetworkFilter: &rules.NetworkFilterDefinition{
						BPFFilter: "port 53",
						Policy:    "drop",
						Scope:     "process",
					},
				},
			},
			Tags: map[string]string{
				"remediation_rule": "true",
				"agent_event_id":   "AZoIdt0EAAAbKF9Rg_4IIJ",
				"creator_uuid":     "b6497050-10b2-11f0-a294-324b18620407",
				"creator_name":     "Allan Turing",
				"creator_handle":   "allan.turing@example.com",
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkRawPacketEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("kill-remediation-status", func(t *testing.T) {
		testFile, _, err := test.Path("test-kill-remediation")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		err = test.GetEventSent(t, func() error {
			ch := make(chan bool, 1)

			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				cmd := exec.CommandContext(timeoutCtx, syscallTester, "open", testFile, ";", "sleep", "1", ";")
				_ = cmd.Run()

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
		}, time.Second*5, "kill_remediation")

		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("remediation_status")
			if msg == nil {
				return errors.New("not found")
			}

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_id`); err != nil || el != "remediation_status" {
					t.Errorf("agent.rule_id should be 'remediation_status': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.event_type`); err != nil || el != "remediation_status" {
					t.Errorf("event_type should be 'remediation_status': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.remediation_action`); err != nil || el != "kill" {
					t.Errorf("rule_action should be 'kill': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.status`); err != nil || el != "performed" {
					t.Errorf("status should be 'performed': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.scope`); err != nil || el != "process" {
					t.Errorf("scope should be 'process': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.process.pid`); err != nil || el == nil {
					t.Errorf("process.pid not found: %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.rule_tags.remediation_rule`); err != nil || el != "true" {
					t.Errorf("rule_tags.remediation_rule should be 'true': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.rule_tags.agent_event_id`); err != nil || el != "AZoIdt0EAAAbKF9Rg_3TKKJ" {
					t.Errorf("rule_tags.agent_event_id should be 'AZoIdt0EAAAbKF9Rg_3TKKJ': %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
	t.Run("kill-no-tags", func(t *testing.T) {
		testFile, _, err := test.Path("test-kill-no-tags")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		err = test.GetEventSent(t, func() error {
			ch := make(chan bool, 1)

			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				cmd := exec.CommandContext(timeoutCtx, syscallTester, "open", testFile, ";", "sleep", "1", ";")
				_ = cmd.Run()

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
		}, time.Second*5, "kill_no_tags")

		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("remediation_status")
			if msg == nil {
				return errors.New("not found")
			}

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_id`); err != nil || el != "remediation_status" {
					t.Errorf("agent.rule_id should be 'remediation_status': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.event_type`); err != nil || el != "remediation_status" {
					t.Errorf("event_type should be 'remediation_status': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.remediation_action`); err != nil || el != "kill" {
					t.Errorf("rule_action should be 'kill': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.status`); err != nil || el != "performed" {
					t.Errorf("status should be 'performed': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.scope`); err != nil || el != "process" {
					t.Errorf("scope should be 'process': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.process.pid`); err != nil || el == nil {
					t.Errorf("process.pid not found: %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

	t.Run("network-isolation-remediation-status", func(t *testing.T) {
		err = test.GetEventSent(t, func() error {
			cmd := exec.Command("sleep", "123")
			if err := cmd.Start(); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)

			if cmd.Process != nil {
				cmd.Process.Kill()
			}

			return nil
		}, func(_ *rules.Rule, _ *model.Event) bool {
			return true
		}, time.Second*5, "network_remediation")

		if err != nil {
			t.Error(err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("remediation_status")
			if msg == nil {
				return errors.New("not found")
			}

			jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
				if el, err := jsonpath.JsonPathLookup(obj, `$.agent.rule_id`); err != nil || el != "remediation_status" {
					t.Errorf("agent.rule_id should be 'remediation_status': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.event_type`); err != nil || el != "remediation_status" {
					t.Errorf("event_type should be 'remediation_status': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.remediation_action`); err != nil || el != sprobe.RemediationTypeNetworkIsolationStr {
					t.Errorf("rule_action should be 'network_isolation': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.status`); err != nil || el != "performed" {
					t.Errorf("status should be 'performed': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.scope`); err != nil || el != "process" {
					t.Errorf("scope should be 'process': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.process.pid`); err != nil || el == nil {
					t.Errorf("process.pid not found: %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.rule_tags.remediation_rule`); err != nil || el != "true" {
					t.Errorf("rule_tags.remediation_rule should be 'true': %s => %v", string(msg.Data), err)
				}

				if el, err := jsonpath.JsonPathLookup(obj, `$.rule_tags.agent_event_id`); err != nil || el != "AZoIdt0EAAAbKF9Rg_4IIJ" {
					t.Errorf("rule_tags.agent_event_id should be 'AZoIdt0EAAAbKF9Rg_4IIJ': %s => %v", string(msg.Data), err)
				}
			})

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

}
