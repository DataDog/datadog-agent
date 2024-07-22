// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestCGroupID(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping cgroup ID test in docker")
	}

	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_container_flags",
			Expression: `open.file.path == "{{.Root}}/test-open" && cgroup.id == "cg1"`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	cgroupPath, err := createCGroup("cg1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cgroupPath)

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	test.Run(t, "cgroup-id", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))

		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_container_flags")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldEqual(t, event, "container.id", "")
			assertFieldEqual(t, event, "container.runtime", "")
			assert.Equal(t, containerutils.CGroupFlags(0), event.CGroupContext.CGroupFlags)
			assertFieldEqual(t, event, "cgroup.id", "cg1")

			test.validateOpenSchema(t, event)
		})
	})
}
