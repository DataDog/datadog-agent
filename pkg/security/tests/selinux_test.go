// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"gotest.tools/assert"
)

func TestSELinux(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_enforce",
			Expression: `selinux.file.name == "enforce"`,
		},
		{
			ID:         "test_selinux_write_bool",
			Expression: `selinux.file.name == "selinuxuser_ping"`,
		},
	}

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if cmd := exec.Command("sudo", "-n", "sestatus"); cmd.Run() != nil {
		t.Skipf("SELinux is not available, skipping tests")
	}

	t.Run("setenforce", func(t *testing.T) {
		if cmd := exec.Command("sudo", "-n", "setenforce", "0"); cmd.Run() != nil {
			t.Errorf("Failed to run setenforce")
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_enforce")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			fileName := "/sys/fs/selinux/enforce"
			assertFieldEqual(t, event, "selinux.file.path", fileName, "wrong file path")
			assertFieldEqual(t, event, "selinux.file.name", "enforce", "wrong file name")
			assertFieldEqual(t, event, "selinux.file.inode", int(getInode(t, fileName)), "wrong inode")

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})

	t.Run("setsebool", func(t *testing.T) {
		if cmd := exec.Command("sudo", "-n", "setsebool", "selinuxuser_ping", "off"); cmd.Run() != nil {
			t.Errorf("Failed to run setsebool")
		}

		event, rule, err := test.GetEvent()
		t.Log(ppJSON(event))
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_write_bool")
			assert.Equal(t, event.SELinux.File.BasenameStr, "selinuxuser_ping", "wrong bool name")

			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			fileName := "/sys/fs/selinux/booleans/selinuxuser_ping"
			assertFieldEqual(t, event, "selinux.file.path", fileName, "wrong file path")
			assertFieldEqual(t, event, "selinux.file.name", "selinuxuser_ping", "wrong file name")
			assertFieldEqual(t, event, "selinux.file.inode", int(getInode(t, fileName)), "wrong inode")

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}

		if cmd := exec.Command("sudo", "-n", "setsebool", "selinuxuser_ping", "on"); cmd.Run() != nil {
			t.Errorf("Failed to reset setsebool value")
		}
	})
}
