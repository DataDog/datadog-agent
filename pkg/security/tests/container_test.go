// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestContainerCreatedAt(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "OpenSUSE 15.3 kernel", func(kv *kernel.Version) bool {
		// because of some strange btrfs subvolume error
		return kv.IsOpenSUSELeap15_3Kernel()
	})

	checkKernelCompatibility(t, "ContainerCreatedAt test not consistent on CentOS7", func(kv *kernel.Version) bool {
		return kv.IsRH7Kernel()
	})

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_created_at",
			Expression: `process.container.id != "" && container.created_at < 3s && open.file.path == "{{.Root}}/test-open"`,
		},
		{
			ID:         "test_container_created_at_delay",
			Expression: `process.container.id != "" && container.created_at > 3s && open.file.path == "{{.Root}}/test-open-delay"`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	testFileDelay, _, err := test.Path("test-open-delay")
	if err != nil {
		t.Fatal(err)
	}

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu", "")
	if err != nil {
		t.Fatalf("failed to start docker wrapper: %v", err)
	}

	dockerWrapper.Run(t, "container-created-at", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFile}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_container_created_at")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")

			test.validateOpenSchema(t, event)
		}, "test_container_created_at")
	})

	dockerWrapper.Run(t, "container-created-at-delay", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFileDelay}, nil) // shouldn't trigger an event
			if err := cmd.Run(); err != nil {
				return err
			}
			time.Sleep(3 * time.Second)
			cmd = cmdFunc("touch", []string{testFileDelay}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_container_created_at_delay")
			assertFieldEqual(t, event, "open.file.path", testFileDelay)
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")
			createdAtNano, _ := event.GetFieldValue("process.container.created_at")
			createdAt := time.Unix(0, int64(createdAtNano.(int)))
			assert.True(t, time.Since(createdAt) > 3*time.Second)

			test.validateOpenSchema(t, event)
		}, "test_container_created_at_delay")
	})
}

func TestContainerVariables(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_set_variable",
			Expression: `process.container.id != "" && open.file.path == "{{.Root}}/test-open"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "container",
						Value: 1,
						Name:  "foo",
					},
				},
			},
		},
		{
			ID:         "test_container_check_variable",
			Expression: `process.container.id != "" && open.file.path == "{{.Root}}/test-open2" && ${container.foo} == 1`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	testFile2, _, err := test.Path("test-open2")
	if err != nil {
		t.Fatal(err)
	}

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu", "")
	if err != nil {
		t.Fatalf("failed to start docker wrapper: %v", err)
	}

	dockerWrapper.Run(t, "container-variables", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFile}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_container_set_variable")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")

			test.validateOpenSchema(t, event)
		}, "test_container_set_variable")

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFile2}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_container_check_variable")
			assertFieldEqual(t, event, "open.file.path", testFile2)
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")

			test.validateOpenSchema(t, event)
		}, "test_container_check_variable")
	})
}

func TestContainerVariablesReleased(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_set_variable",
			Expression: `process.container.id != "" && open.file.path == "/tmp/test-open"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "container",
						Value: 999,
						Name:  "bar",
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

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu", "")
	if err != nil {
		t.Fatalf("failed to create docker wrapper: %v", err)
	}
	_, err = dockerWrapper.start()
	if err != nil {
		t.Fatalf("failed to start docker wrapper: %v", err)
	}

	test.WaitSignalFromRule(t, func() error {
		return dockerWrapper.Command("touch", []string{"/tmp/test-open"}, nil).Run()
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_container_set_variable")
		assertFieldEqual(t, event, "open.file.path", "/tmp/test-open")
		assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")

		variables := test.ruleEngine.GetRuleSet().GetScopedVariables(rules.ScopeContainer, "bar")
		assert.NotNil(t, variables)
		assert.Contains(t, variables, event.ProcessContext.Process.ContainerContext.Hash())
		variable, ok := variables[event.ProcessContext.Process.ContainerContext.Hash()]
		assert.True(t, ok)
		value, ok := variable.GetValue()
		assert.True(t, ok)
		assert.Equal(t, 999, value.(int))
	}, "test_container_set_variable")

	_, err = dockerWrapper.stop()
	if err != nil {
		t.Fatalf("failed to stop docker wrapper: %v", err)
	}

	time.Sleep(500 * time.Millisecond) // wait just a bit of time for the container to be released

	variables := test.ruleEngine.GetRuleSet().GetScopedVariables(rules.ScopeContainer, "bar")
	assert.NotNil(t, variables)
	assert.Len(t, variables, 0)
}
