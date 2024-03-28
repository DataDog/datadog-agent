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

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestUserGroup(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

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
		name  string
		cmd   []string
		rules []string
	}

	distroTests := []struct {
		name         string
		testCommands []testCommand
	}{
		{
			name: "ubuntu",
			testCommands: []testCommand{
				{
					name:  "addgroup",
					cmd:   []string{"/usr/sbin/groupadd", "--gid", "1999", "testgroup"},
					rules: []string{"refresh_user_cache"},
				},
				{
					name:  "adduser",
					cmd:   []string{"/usr/sbin/useradd", "--gid", "1999", "--uid", "1999", "testuser"},
					rules: []string{"refresh_user_cache"},
				},
				{
					name:  "user-resolution",
					cmd:   []string{"/usr/bin/su", "--command", "/usr/bin/touch /tmp/test", "testuser"},
					rules: []string{"test_rule_user"},
				},
				{
					name:  "group-resolution",
					cmd:   []string{"/usr/bin/su", "-g", "testgroup", "--command", "/usr/bin/touch /tmp/test2"},
					rules: []string{"test_rule_group"},
				},
			},
		},
		{
			name: "centos",
			testCommands: []testCommand{
				{
					name:  "addgroup",
					cmd:   []string{"/usr/sbin/groupadd", "--gid", "1999", "testgroup"},
					rules: []string{"refresh_user_cache"},
				},
				{
					name:  "adduser",
					cmd:   []string{"/usr/sbin/useradd", "--gid", "1999", "--uid", "1999", "testuser"},
					rules: []string{"refresh_user_cache"},
				},
				{
					name:  "user-resolution",
					cmd:   []string{"/usr/bin/su", "--command", "/usr/bin/touch /tmp/test", "testuser"},
					rules: []string{"test_rule_user"},
				},
				{
					name:  "group-resolution",
					cmd:   []string{"/usr/bin/su", "-g", "testgroup", "--command", "/usr/bin/touch /tmp/test2"},
					rules: []string{"test_rule_group"},
				},
			},
		},
		{
			name: "alpine",
			testCommands: []testCommand{
				{
					name:  "addgroup",
					cmd:   []string{"/usr/sbin/addgroup", "--gid", "1999", "testgroup"},
					rules: []string{"refresh_user_cache"},
				},
				{
					name: "adduser",
					cmd:  []string{"/usr/sbin/adduser", "-D", "-G", "testgroup", "-u", "1999", "testuser"},
					// busybox 'adduser' calls addgroup, that updates /etc/group
					rules: []string{"refresh_user_cache", "refresh_user_cache"},
				},
				{
					name:  "user-resolution",
					cmd:   []string{"/bin/su", "testuser", "-c", "/bin/busybox touch /tmp/test"},
					rules: []string{"test_rule_user"},
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for _, distroTest := range distroTests {
		t.Run(distroTest.name, func(t *testing.T) {
			dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), distroTest.name)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := dockerWrapper.start(); err != nil {
				t.Fatal(err)
			}
			defer dockerWrapper.stop()

			for _, testCommand := range distroTest.testCommands {
				i := 0
				dockerWrapper.RunTest(t, testCommand.name, func(t *testing.T, kind wrapperType, cmdFunc func(bin string, args, env []string) *exec.Cmd) {
					test.WaitSignals(t, func() error {
						return cmdFunc(testCommand.cmd[0], testCommand.cmd[1:], nil).Run()
					}, func(event *model.Event, rule *rules.Rule) error {
						assertTriggeredRule(t, rule, testCommand.rules[i])
						i++
						if i < len(testCommand.rules) {
							return errSkipEvent
						}
						return nil
					})
				})
			}
		})
	}
}
