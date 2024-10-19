// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestLoginUID(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{
		{
			ID:         "auid_exec",
			Expression: `exec.auid >= 1000 && exec.auid != AUDIT_AUID_UNSET && exec.file.name == "syscall_go_tester"`,
		},
		{
			ID:         "auid_open",
			Expression: `open.file.path == "/tmp/test-auid" && open.flags & O_CREAT != 0 && process.auid == 1005 && process.file.name == "syscall_go_tester"`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	var dockerInstance *dockerCmdWrapper
	dockerInstance, err = test.StartADocker()
	if err != nil {
		t.Fatalf("failed to start a Docker instance: %v", err)
	}
	defer dockerInstance.stop()

	t.Run("open", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{
				"-login-uid-test",
				"-login-uid-event-type", "open",
				"-login-uid-path", "/tmp/test-auid",
				"-login-uid-value", "1005",
			}

			cmd := dockerInstance.Command(goSyscallTester, args, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, "auid_open", rule.ID, "wrong rule triggered")
		})
	})

	t.Run("exec", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{
				"-login-uid-test",
				"-login-uid-event-type", "exec",
				"-login-uid-path", goSyscallTester,
				"-login-uid-value", "1005",
			}

			cmd := dockerInstance.Command(goSyscallTester, args, []string{})
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("command exited with an error: out:'%s' err:'%v'", string(out), err)
				return err
			}

			t.Logf("test out: %s\n", string(out))

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "exec", event.GetType(), "wrong event type")
			assert.Equal(t, "auid_exec", rule.ID, "wrong rule triggered")
		})
	})
}
