// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestVariableAnyField(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID: "test_rule_field_variable",
		// TODO(lebauce): should infer event type from variable usage
		Expression: `open.file.path != "" && "%{open.file.path}:foo" == "{{.Root}}/test-open:foo"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename1 string

	test.WaitSignalFromRule(t, func() error {
		filename1, _, err = test.Create("test-open")
		return err
	}, func(_ *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_field_variable", rule.ID, "wrong rule triggered")
	}, "test_rule_field_variable")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename1)
}

func TestVariablesGetVariables(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "sleep_rule",
			Expression: `exec.file.path == "/usr/bin/sleep" && exec.argv in ["999"]`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Name:  "sleep_container_id",
						Scope: "container",
						Field: "process.container.id",
					},
				},
				{
					Set: &rules.SetDefinition{
						Name:  "sleep_cgroup_id",
						Scope: "cgroup",
						Field: "process.cgroup.id",
					},
				},
				{
					Set: &rules.SetDefinition{
						Name:  "sleep_process_id",
						Scope: "process",
						Field: "process.pid",
					},
				},
				{
					Set: &rules.SetDefinition{
						Name:   "sleep_global",
						Field:  "exec.file.path",
						Append: true,
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
		t.Fatalf("failed to start docker wrapper: %v", err)
	}
	_, err = dockerWrapper.start()
	if err != nil {
		t.Fatalf("failed to start docker wrapper: %v", err)
	}
	t.Cleanup(func() {
		output, err := dockerWrapper.stop()
		if err != nil {
			t.Errorf("failed to stop docker wrapper: %v\n%s", err, string(output))
		}
	})

	t.Run("get-variables", func(t *testing.T) {
		var capturedContainerID string
		var capturedCGroupID string
		var capturedPID uint32

		test.WaitSignalFromRule(t, func() error {
			return dockerWrapper.Command("/usr/bin/sleep", []string{"999"}, nil).Start()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "sleep_rule")
			// Capture values from the event
			capturedContainerID = string(event.ProcessContext.Process.ContainerContext.ContainerID)
			capturedCGroupID = string(event.ProcessContext.Process.CGroup.CGroupID)
			capturedPID = event.ProcessContext.Process.Pid

			// Validate we got the expected values
			assertFieldNotEmpty(t, event, "process.container.id", "container ID shouldn't be empty")
			assertFieldNotEmpty(t, event, "process.cgroup.id", "cgroup ID shouldn't be empty")
			assert.NotZero(t, capturedPID, "process PID should not be zero")
		}, "sleep_rule")

		// Validate that we captured the values
		if capturedContainerID == "" {
			t.Fatal("Failed to capture container ID from event")
		}
		if capturedCGroupID == "" {
			t.Fatal("Failed to capture cgroup ID from event")
		}
		if capturedPID == 0 {
			t.Fatal("Failed to capture PID from event")
		}

		if test.cws == nil {
			t.Fatal("CWS consumer is nil")
		}

		seclVariables := test.cws.APIServer().GetSECLVariables()
		if seclVariables == nil {
			t.Fatal("GetSECLVariables returned nil")
		}

		globalVarName := "sleep_global"
		globalVar, found := seclVariables[globalVarName]
		if !found {
			t.Errorf("Global variable '%s' not found in SECL variables", globalVarName)
		} else {
			assert.Equal(t, globalVarName, globalVar.Name, "Global variable name mismatch")
			assert.Contains(t, globalVar.Value, "/usr/bin/sleep", "Global variable should contain the sleep path")
		}

		containerVarName := fmt.Sprintf("container.sleep_container_id.%s", capturedContainerID)
		containerVar, found := seclVariables[containerVarName]
		if !found {
			t.Errorf("Container variable '%s' not found in SECL variables", containerVarName)
		} else {
			assert.Equal(t, containerVarName, containerVar.Name, "Container variable name mismatch")
			assert.Equal(t, capturedContainerID, containerVar.Value, "Container variable should match container ID")
		}

		cgroupVarName := fmt.Sprintf("cgroup.sleep_cgroup_id.%s", capturedCGroupID)
		cgroupVar, found := seclVariables[cgroupVarName]
		if !found {
			t.Errorf("CGroup variable '%s' not found in SECL variables", cgroupVarName)
		} else {
			assert.Equal(t, cgroupVarName, cgroupVar.Name, "CGroup variable name mismatch")
			assert.Equal(t, capturedCGroupID, cgroupVar.Value, "CGroup variable should match cgroup ID")
		}

		processVarName := fmt.Sprintf("process.sleep_process_id.%d", capturedPID)
		processVar, found := seclVariables[processVarName]
		if !found {
			t.Errorf("Process variable '%s' not found in SECL variables", processVarName)
		} else {
			assert.Equal(t, processVarName, processVar.Name, "Process variable name mismatch")
			expectedPIDStr := fmt.Sprintf("%d", capturedPID)
			assert.Equal(t, expectedPIDStr, processVar.Value, "Process variable should match PID")
		}
	})
}
