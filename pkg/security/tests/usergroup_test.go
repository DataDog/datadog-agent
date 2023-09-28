// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestUserGroup(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_user",
			Expression: `open.file.path == "/tmp/test" && open.flags & O_CREAT != 0 && open.file.uid == 1999 && open.file.user == "testuser"`,
		},
		{
			ID:         "test_rule_group",
			Expression: `open.file.path == "/tmp/test2" && open.flags & O_CREAT != 0 && open.file.gid == 1999 && open.file.group == "testgroup"`,
		},
	}

	type testCommand struct {
		name string
		cmd  []string
		rule string
	}

	distroTests := []struct {
		name         string
		testCommands []testCommand
	}{
		{
			name: "ubuntu",
			testCommands: []testCommand{
				{
					name: "addgroup",
					cmd:  []string{"/usr/sbin/groupadd", "--gid", "1999", "testgroup"},
					rule: "refresh_user_cache",
				},
				{
					name: "adduser",
					cmd:  []string{"/usr/sbin/useradd", "--gid", "1999", "--uid", "1999", "testuser"},
					rule: "refresh_user_cache",
				},
				{
					name: "user-resolution",
					cmd:  []string{"/usr/bin/su", "--command", "/usr/bin/touch /tmp/test", "testuser"},
					rule: "test_rule_user",
				},
				{
					name: "group-resolution",
					cmd:  []string{"/usr/bin/su", "-g", "testgroup", "--command", "/usr/bin/touch /tmp/test2"},
					rule: "test_rule_group",
				},
			},
		},
		{
			name: "centos",
			testCommands: []testCommand{
				{
					name: "addgroup",
					cmd:  []string{"/usr/sbin/groupadd", "--gid", "1999", "testgroup"},
					rule: "refresh_user_cache",
				},
				{
					name: "adduser",
					cmd:  []string{"/usr/sbin/useradd", "--gid", "1999", "--uid", "1999", "testuser"},
					rule: "refresh_user_cache",
				},
				{
					name: "user-resolution",
					cmd:  []string{"/usr/bin/su", "--command", "/usr/bin/touch /tmp/test", "testuser"},
					rule: "test_rule_user",
				},
				{
					name: "group-resolution",
					cmd:  []string{"/usr/bin/su", "-g", "testgroup", "--command", "/usr/bin/touch /tmp/test2"},
					rule: "test_rule_group",
				},
			},
		},
		{
			name: "alpine",
			testCommands: []testCommand{
				{
					name: "addgroup",
					cmd:  []string{"/usr/sbin/addgroup", "--gid", "1999", "testgroup"},
					rule: "refresh_user_cache",
				},
				{
					name: "adduser",
					cmd:  []string{"/usr/sbin/adduser", "-D", "-G", "testgroup", "-u", "1999", "testuser"},
					rule: "refresh_user_cache",
				},
				{
					// busybox 'adduser' calls addgroup, that updates /etc/group
					name: "add-user-to-group",
					cmd:  []string{"/bin/ls"},
					rule: "refresh_user_cache",
				},
				{
					name: "user-resolution",
					cmd:  []string{"/bin/su", "testuser", "-c", "/bin/busybox touch /tmp/test"},
					rule: "test_rule_user",
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for _, distroTest := range distroTests {
		dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), distroTest.name)
		if err != nil {
			t.Skipf("Skipping user group tests: Docker not available: %s", err)
			return
		}

		if _, err := dockerWrapper.start(); err != nil {
			t.Fatal(err)
		}
		defer dockerWrapper.stop()

		for _, testCommand := range distroTest.testCommands {
			dockerWrapper.RunTest(t, distroTest.name+"-"+testCommand.name, func(t *testing.T, kind wrapperType, cmdFunc func(bin string, args, env []string) *exec.Cmd) {
				test.WaitSignal(t, func() error {
					return cmdFunc(testCommand.cmd[0], testCommand.cmd[1:], nil).Run()
				}, func(event *model.Event, rule *rules.Rule) {
					assertTriggeredRule(t, rule, testCommand.rule)
				})
			})
		}
	}
}
