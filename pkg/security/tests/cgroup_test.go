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
	"slices"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type testCGroup struct {
	cgroupPath         string
	previousCGroupPath string
}

func (cg *testCGroup) enter() error {
	return os.WriteFile(cg.cgroupPath+"/cgroup.procs", []byte(strconv.Itoa(os.Getpid())), 0700)
}

func (cg *testCGroup) leave(t *testing.T) {
	if err := os.WriteFile("/sys/fs/cgroup"+cg.previousCGroupPath+"/cgroup.procs", []byte(strconv.Itoa(os.Getpid())), 0700); err != nil {
		if err := os.WriteFile("/sys/fs/cgroup/systemd"+cg.previousCGroupPath+"/cgroup.procs", []byte(strconv.Itoa(os.Getpid())), 0700); err != nil {
			t.Log(err)
			return
		}
	}
}

func (cg *testCGroup) remove(t *testing.T) {
	if err := os.Remove(cg.cgroupPath); err != nil {
		if content, err := os.ReadFile(cg.cgroupPath + "/cgroup.procs"); err == nil {
			t.Logf("Processes in cgroup: %s", string(content))
		}
	}
}

func (cg *testCGroup) create() error {
	return os.MkdirAll(cg.cgroupPath, 0700)
}

func newCGroup(name, kind string) (*testCGroup, error) {
	cgs, err := utils.GetProcControlGroups(uint32(os.Getpid()), uint32(os.Getpid()))
	if err != nil {
		return nil, err
	}

	var previousCGroupPath string
	for _, cg := range cgs {
		if len(cg.Controllers) == 1 && cg.Controllers[0] == "" {
			previousCGroupPath = cg.Path
			break
		}
		if previousCGroupPath == "" {
			previousCGroupPath = cg.Path
		} else if previousCGroupPath == "/" {
			previousCGroupPath = cg.Path
		}
		if slices.Contains(cg.Controllers, kind) || slices.Contains(cg.Controllers, "name="+kind) {
			previousCGroupPath = cg.Path
			break
		}
	}

	cgroupPath := "/sys/fs/cgroup/" + kind + "/" + name
	cg := &testCGroup{
		previousCGroupPath: previousCGroupPath,
		cgroupPath:         cgroupPath,
	}

	return cg, nil
}

func TestCGroup(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping cgroup ID test in docker")
	}

	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_id",
			Expression: `open.file.path == "{{.Root}}/test-open" && process.cgroup.id =~ "*/cg1"`,
		},
		{
			ID:         "test_cgroup_systemd",
			Expression: `open.file.path == "{{.Root}}/test-open2" && process.cgroup.id == "/system.slice/cws-test.service"`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testCGroup, err := newCGroup("cg1", "systemd")
	if err != nil {
		t.Fatal(err)
	}

	if err := testCGroup.create(); err != nil {
		t.Fatal(err)
	}
	defer testCGroup.remove(t)

	if err := testCGroup.enter(); err != nil {
		t.Fatal(err)
	}
	defer testCGroup.leave(t)

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	testFile2, _, err := test.Path("test-open2")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cgroup-id", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))

		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_id")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldEqual(t, event, "process.container.id", "")
			assertFieldIsOneOf(t, event, "process.cgroup.id", []string{"/cg1", "/systemd/cg1"})
			assertFieldIsOneOf(t, event, "process.cgroup.version", []int{1, 2})

			test.validateOpenSchema(t, event)
		}, "test_cgroup_id")
	})

	t.Run("systemd", func(t *testing.T) {

		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO(lebauce): On the systems, systemd service creation doesn't trigger a cprocs write
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel() || kv.IsOpenSUSELeapKernel()
		})

		test.WaitSignalFromRule(t, func() error {
			serviceUnit := `[Service]
Type=oneshot
ExecStart=/usr/bin/touch ` + testFile2
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
			assertFieldNotEqual(t, event, "process.cgroup.id", "")

			test.validateOpenSchema(t, event)
		}, "test_cgroup_systemd")
	})
}

func TestCGroupSnapshot(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping cgroup ID test in docker")
	}

	SkipIfNotAvailable(t)

	cfs := utils.DefaultCGroupFS()

	_, cgroupContext, _, err := cfs.FindCGroupContext(uint32(os.Getpid()), uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}

	testCGroup, err := newCGroup("cg2", "systemd")
	if err != nil {
		t.Fatal(err)
	}

	if err := testCGroup.create(); err != nil {
		t.Fatal(err)
	}
	defer testCGroup.remove(t)

	if err := testCGroup.enter(); err != nil {
		t.Fatal(err)
	}
	defer testCGroup.leave(t)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	var testsuiteStats unix.Stat_t
	if err := unix.Stat(executable, &testsuiteStats); err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_snapshot",
			Expression: `open.file.path == "{{.Root}}/test-open" && process.cgroup.id != ""`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	testFile, _, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	var syscallTesterStats unix.Stat_t
	if err := unix.Stat(syscallTester, &syscallTesterStats); err != nil {
		t.Fatal(err)
	}

	var cmd *exec.Cmd
	test.WaitSignalFromRule(t, func() error {
		cmd = exec.Command(syscallTester, "open", testFile)
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}

		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_cgroup_snapshot")
		test.validateOpenSchema(t, event)

		testsuiteEntry := p.Resolvers.ProcessResolver.Get(uint32(os.Getpid()))
		syscallTesterEntry := p.Resolvers.ProcessResolver.Get(uint32(cmd.Process.Pid))
		assert.NotNil(t, testsuiteEntry)
		assert.NotNil(t, syscallTesterEntry)

		// Check that testsuite has changed cgroup since its start
		assert.NotEqual(t, cgroupContext.CGroupID, testsuiteEntry.CGroup.CGroupID)
		assert.Equal(t, int(testsuiteEntry.Pid), os.Getpid())

		// Check that both testsuite and syscall tester share the same cgroup
		assert.Equal(t, testsuiteEntry.CGroup.CGroupID, syscallTesterEntry.CGroup.CGroupID)
		assert.Equal(t, testsuiteEntry.CGroup.CGroupPathKey, syscallTesterEntry.CGroup.CGroupPathKey)

		// Check that we have the right cgroup inode
		cgroupFS := utils.DefaultCGroupFS()
		_, _, cgroupSysFSPath, err := cgroupFS.FindCGroupContext(uint32(os.Getpid()), uint32(os.Getpid()))
		if err != nil {
			t.Error(err)
			return
		}

		var stats unix.Stat_t
		if err := unix.Stat(cgroupSysFSPath, &stats); err != nil {
			t.Error(err)
			return
		}
		assert.Equal(t, stats.Ino, testsuiteEntry.CGroup.CGroupPathKey.Inode)

		// Check we filled the kernel maps correctly with the same values than userspace for the testsuite process
		var newEntry *model.ProcessCacheEntry
		ebpfProbe := test.probe.PlatformProbe.(*probe.EBPFProbe)
		ebpfProbe.Resolvers.ProcessResolver.ResolveFromKernelMaps(uint32(os.Getpid()), uint32(os.Getpid()), testsuiteStats.Ino, func(entry *model.ProcessCacheEntry, _ error) {
			newEntry = entry
		})
		assert.NotNil(t, newEntry)
		if newEntry != nil {
			assert.Equal(t, stats.Ino, newEntry.CGroup.CGroupPathKey.Inode)
		}

		// Check we filled the kernel maps correctly with the same values than userspace for the syscall tester process
		newEntry = nil
		ebpfProbe.Resolvers.ProcessResolver.ResolveFromKernelMaps(syscallTesterEntry.Pid, syscallTesterEntry.Pid, syscallTesterStats.Ino, func(entry *model.ProcessCacheEntry, _ error) {
			newEntry = entry
		})
		assert.NotNil(t, newEntry)
		if newEntry != nil {
			assert.Equal(t, stats.Ino, newEntry.CGroup.CGroupPathKey.Inode)
		}
	}, "test_cgroup_snapshot")
}

func TestCGroupVariables(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_set_variable",
			Expression: `process.cgroup.id != "" && open.file.path == "{{.Root}}/test-open"`,
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
			Expression: `process.cgroup.id != "" && open.file.path == "{{.Root}}/test-open2" && ${cgroup.foo} == 1`,
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

	dockerWrapper.Run(t, "cgroup-variables", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFile}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_set_variable")
			assertFieldEqual(t, event, "open.file.path", testFile)
			assertFieldNotEmpty(t, event, "process.cgroup.id", "cgroup id shouldn't be empty")

			test.validateOpenSchema(t, event)
		}, "test_cgroup_set_variable")

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFile2}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_check_variable")
			assertFieldEqual(t, event, "open.file.path", testFile2)
			assertFieldNotEmpty(t, event, "process.cgroup.id", "cgroup id shouldn't be empty")

			test.validateOpenSchema(t, event)
		}, "test_cgroup_check_variable")
	})
}

func TestCGroupVariablesReleased(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_set_variable",
			Expression: `process.cgroup.id != "" && open.file.path == "/tmp/test-open"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope: "cgroup",
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
		assertTriggeredRule(t, rule, "test_cgroup_set_variable")
		assertFieldEqual(t, event, "open.file.path", "/tmp/test-open")
		assertFieldNotEmpty(t, event, "process.cgroup.id", "cgroup id shouldn't be empty")

		variables := test.ruleEngine.GetRuleSet().GetScopedVariables(rules.ScopeCGroup, "bar")
		assert.NotNil(t, variables)
		assert.Contains(t, variables, event.ProcessContext.Process.CGroup.Hash())
		variable, ok := variables[event.ProcessContext.Process.CGroup.Hash()]
		assert.True(t, ok)
		value, ok := variable.GetValue()
		assert.True(t, ok)
		assert.Equal(t, 999, value.(int))
	}, "test_cgroup_set_variable")

	_, err = dockerWrapper.stop()
	if err != nil {
		t.Fatalf("failed to stop docker wrapper: %v", err)
	}

	time.Sleep(500 * time.Millisecond) // wait just a bit of time for the cgroup to be released

	variables := test.ruleEngine.GetRuleSet().GetScopedVariables(rules.ScopeCGroup, "bar")
	assert.NotNil(t, variables)
	assert.Len(t, variables, 0)
}

func TestCGroupWriteEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_cgroup_write_rule",
			Expression: `cgroup_write.file.path != ""`,
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

	testFile, _, err := test.Path("test-cgroup-write")
	if err != nil {
		t.Fatal(err)
	}

	dockerWrapper.Run(t, "cgroup-write-event", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("touch", []string{testFile}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_cgroup_write_rule")
			validateProcessContext(t, event)
		}, "test_cgroup_write_rule")
	})
}
