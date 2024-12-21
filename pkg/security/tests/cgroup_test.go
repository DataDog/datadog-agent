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
	"os/exec"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func createCGroup(name string) (string, error) {
	cgroupPath := "/sys/fs/cgroup/memory/" + name
	if err := os.MkdirAll(cgroupPath, 0700); err != nil {
		return "", err
	}

	if err := os.WriteFile(cgroupPath+"/cgroup.procs", []byte(strconv.Itoa(os.Getpid())), 0700); err != nil {
		return "", err
	}

	return cgroupPath, nil
}

func TestCGroup(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping cgroup ID test in docker")
	}

	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_id",
			Expression: `open.file.path == "{{.Root}}/test-open" && cgroup.id =~ "*/cg1"`, // "/memory/cg1" or "/cg1"
		},
		{
			ID:         "test_cgroup_systemd",
			Expression: `open.file.path == "{{.Root}}/test-open2" && cgroup.id == "/system.slice/cws-test.service"`, // && cgroup.manager == "systemd"
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

	testFile2, _, err := test.Path("test-open2")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cgroup-id", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))

		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_id")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldEqual(t, event, "container.id", "")
			assertFieldEqual(t, event, "container.runtime", "")
			assert.Equal(t, containerutils.CGroupFlags(0), event.CGroupContext.CGroupFlags)
			assertFieldIsOneOf(t, event, "cgroup.id", "/memory/cg1")
			assertFieldIsOneOf(t, event, "cgroup.version", []int{1, 2})

			test.validateOpenSchema(t, event)
		})
	})

	t.Run("systemd", func(t *testing.T) {

		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO(lebauce): On the systems, systemd service creation doesn't trigger a cprocs write
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
		})

		test.WaitSignal(t, func() error {
			serviceUnit := fmt.Sprintf(`[Service]
Type=oneshot
ExecStart=/usr/bin/touch %s`, testFile2)
			if err := os.WriteFile("/etc/systemd/system/cws-test.service", []byte(serviceUnit), 0700); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "start", "cws-test").Run(); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "stop", "cws-test").Run(); err != nil {
				return err
			}
			if err := os.Remove("/etc/systemd/system/cws-test.service"); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_systemd")
			assertFieldEqual(t, event, "open.file.path", testFile2)
			assertFieldEqual(t, event, "cgroup.manager", "systemd")
			assertFieldNotEqual(t, event, "cgroup.id", "")

			test.validateOpenSchema(t, event)
		})
	})

	t.Run("podman", func(t *testing.T) {
		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO(lebauce): On the systems, systemd service creation doesn't trigger a cprocs write
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
		})

		test.WaitSignal(t, func() error {
			serviceUnit := fmt.Sprintf(`[Service]
Type=oneshot
ExecStart=/usr/bin/touch %s`, testFile2)
			if err := os.WriteFile("/etc/systemd/system/cws-test.service", []byte(serviceUnit), 0700); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "start", "cws-test").Run(); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "stop", "cws-test").Run(); err != nil {
				return err
			}
			if err := os.Remove("/etc/systemd/system/cws-test.service"); err != nil {
				return err
			}
			if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_systemd")
			assertFieldEqual(t, event, "open.file.path", testFile2)
			assertFieldEqual(t, event, "cgroup.manager", "systemd")
			assertFieldNotEqual(t, event, "cgroup.id", "")

			test.validateOpenSchema(t, event)
		})
	})
}

func TestCGroupVariables(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_set_variable",
			Expression: `cgroup.id != "" && open.file.path == "{{.Root}}/test-open"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "cgroup",
						Value: 1,
						Name:  "foo",
					},
				},
			},
		},
		{
			ID:         "test_cgroup_check_variable",
			Expression: `cgroup.id != "" && open.file.path == "{{.Root}}/test-open2" && ${cgroup.foo} == 1`,
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
		t.Skip("Skipping created time in containers tests: Docker not available")
		return
	}
	defer dockerWrapper.stop()

	dockerWrapper.Run(t, "cgroup-variables", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("touch", []string{testFile}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_set_variable")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldNotEmpty(t, event, "cgroup.id", "cgroup id shouldn't be empty")

			test.validateOpenSchema(t, event)
		})

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("touch", []string{testFile2}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_check_variable")
			assertFieldEqual(t, event, "open.file.path", testFile2)
			assertFieldNotEmpty(t, event, "cgroup.id", "cgroup id shouldn't be empty")

			test.validateOpenSchema(t, event)
		})
	})
}
