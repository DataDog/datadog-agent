// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/avast/retry-go"
	"github.com/pkg/errors"
	"github.com/syndtr/gocapability/capability"
	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`process.user != "" && process.file.name == "%s" && open.file.path == "{{.Root}}/test-process"`, path.Base(executable)),
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Create("test-process")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	_, rule, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		assertTriggeredRule(t, rule, "test_rule")
	}
}

func TestProcessContext(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_inode",
			Expression: `open.file.path == "{{.Root}}/test-process-context" && open.flags & O_CREAT != 0`,
		},
		{
			ID:         "test_rule_ancestors",
			Expression: `open.file.path == "{{.Root}}/test-process-ancestors" && process.ancestors[_].file.name in ["dash", "bash"]`,
		},
		{
			ID:         "test_rule_pid1",
			Expression: `open.file.path == "{{.Root}}/test-process-pid1" && process.ancestors[_].pid == 1`,
		},
		{
			ID:         "test_rule_args_envs",
			Expression: `exec.file.name == "ls" && exec.args in [~"*-al*"] && exec.envs in [~"LD_*"]`,
		},
		{
			ID:         "test_rule_argv",
			Expression: `exec.argv in ["-ll"]`,
		},
		{
			ID:         "test_rule_args_flags",
			Expression: `exec.args_flags == "l" && exec.args_flags == "s" && exec.args_flags == "escape"`,
		},
		{
			ID:         "test_rule_args_options",
			Expression: `exec.args_options in ["block-size=123"]`,
		},
		{
			ID:         "test_rule_tty",
			Expression: `open.file.path == "{{.Root}}/test-process-tty" && open.flags & O_CREAT == 0`,
		},
	}

	var rhel7 bool

	kv, err := probe.NewKernelVersion()
	if err == nil {
		rhel7 = kv.IsRH7Kernel()
	}

	test, err := newTestModule(nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	which := func(name string) string {
		executable := "/usr/bin/" + name
		if resolved, err := os.Readlink(executable); err == nil {
			executable = resolved
		} else {
			if os.IsNotExist(err) {
				executable = "/bin/" + name
			}
		}
		return executable
	}

	t.Run("inode", func(t *testing.T) {
		testFile, _, err := test.Path("test-process-context")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertFieldEqual(t, event, "process.file.path", executable)
			assert.Equal(t, event.ResolveProcessCacheEntry().FileFields.Inode, getInode(t, executable), "wrong inode")

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "process.file.container_path")
			}
		}
	})

	test.Run(t, "args-envs", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al", "--password", "secret", "--custom", "secret"}
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}
		cmd := cmdFunc("ls", args, envs)
		_ = cmd.Run()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			// args
			args, err := event.GetFieldValue("exec.args")
			if err != nil || len(args.(string)) == 0 {
				t.Error("not able to get args")
			}

			contains := func(s string) bool {
				for _, arg := range strings.Split(args.(string), " ") {
					if s == arg {
						return true
					}
				}
				return false
			}

			if !contains("-al") || !contains("--password") || !contains("--custom") {
				t.Error("arg not found")
			}

			// envs
			envs, err := event.GetFieldValue("exec.envs")
			if err != nil || len(envs.([]string)) == 0 {
				t.Error("not able to get envs")
			}

			contains = func(s string) bool {
				for _, env := range envs.([]string) {
					if s == env {
						return true
					}
				}
				return false
			}

			if !contains("LD_LIBRARY_PATH") {
				t.Errorf("env not found: %v", event)
			}

			// trigger serialization to test scrubber
			str := event.String()

			if !strings.Contains(str, "password") || !strings.Contains(str, "custom") {
				t.Error("args not serialized")
			}

			if strings.Contains(str, "secret") || strings.Contains(str, "/tmp/lib") {
				t.Error("secret or env values exposed")
			}

			if !rhel7 && !validateExecSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment || kind == dockerWrapperType {
				testContainerPath(t, event, "exec.file.container_path")
			}
		}
	})

	t.Run("argv", func(t *testing.T) {
		lsExecutable := which("ls")
		cmd := exec.Command(lsExecutable, "-ll")
		_ = cmd.Run()

		_, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_argv")
		}
	})

	t.Run("args-flags", func(t *testing.T) {
		lsExecutable := which("ls")
		cmd := exec.Command(lsExecutable, "-ls", "--escape")
		_ = cmd.Run()

		_, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_args_flags")
		}
	})

	t.Run("args-options", func(t *testing.T) {
		lsExecutable := which("ls")
		cmd := exec.Command(lsExecutable, "--block-size", "123")
		_ = cmd.Run()

		_, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_args_options")
		}
	})

	test.Run(t, "args-overflow", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al"}
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}

		// size overflow
		var long string
		for i := 0; i != 1024; i++ {
			long += "a"
		}
		args = append(args, long)

		cmd := cmdFunc("ls", args, envs)
		_ = cmd.Run()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			args, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(args.(string), " ")
			assert.Equal(t, len(argv), 2, "incorrect number of args: %s", argv)
			assert.Equal(t, strings.HasSuffix(argv[1], "..."), true, "args not truncated")
		}

		// number of args overflow
		nArgs, args := 200, []string{"-al"}
		for i := 0; i != nArgs; i++ {
			args = append(args, "aaa")
		}

		cmd = cmdFunc("ls", args, envs)
		_ = cmd.Run()

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			args, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(args.(string), " ")
			n := len(argv)
			if n == 0 || n > nArgs {
				t.Errorf("incorrect number of args %d: %s", n, args.(string))
			}

			if argv[n-1] != "..." {
				t.Errorf("arg not truncated: %s", args.(string))
			}

			if !rhel7 && !validateExecSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment || kind == dockerWrapperType {
				testContainerPath(t, event, "exec.file.container_path")
			}
		}
	})

	t.Run("tty", func(t *testing.T) {
		testFile, _, err := test.Path("test-process-tty")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		executable := "/usr/bin/tail"
		if resolved, err := os.Readlink(executable); err == nil {
			executable = resolved
		} else {
			if os.IsNotExist(err) {
				executable = "/bin/tail"
			}
		}

		var wg sync.WaitGroup

		go func() {
			wg.Add(1)
			defer wg.Done()

			time.Sleep(2 * time.Second)
			cmd := exec.Command("script", "/dev/null", "-c", executable+" -f "+testFile)
			if err := cmd.Start(); err != nil {
				t.Error(err)
				return
			}
			time.Sleep(2 * time.Second)

			cmd.Process.Kill()
			cmd.Wait()
		}()
		defer wg.Wait()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertFieldEqual(t, event, "process.file.path", executable)

			if name, _ := event.GetFieldValue("process.tty_name"); !strings.HasPrefix(name.(string), "pts") {
				t.Errorf("not able to get a tty name: %s\n", name)
			}

			if inode := getInode(t, executable); inode != event.ResolveProcessCacheEntry().FileFields.Inode {
				t.Errorf("expected inode %d, got %d => %+v", event.ResolveProcessCacheEntry().FileFields.Inode, inode, event)
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "process.file.container_path")
			}
		}
	})

	test.Run(t, "ancestors", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		if rhel7 {
			t.Skip()
		}

		testFile, _, err := test.Path("test-process-ancestors")
		if err != nil {
			t.Fatal(err)
		}

		executable := "touch"

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		args := []string{"-c", "$(" + executable + " " + testFile + ")"}

		cmd := cmdFunc("sh", args, nil)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %s", out, err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_ancestors")
			assert.Equal(t, event.ProcessContext.Ancestor.Comm, "sh")

			if !rhel7 && !validateExecSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment || kind == dockerWrapperType {
				testContainerPath(t, event, "process.file.container_path")
				testStringFieldContains(t, event, "process.ancestors.file.container_path", "docker")
			}
		}
	})

	test.Run(t, "pid1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		if rhel7 {
			t.Skip()
		}

		testFile, _, err := test.Path("test-process-pid1")
		if err != nil {
			t.Fatal(err)
		}

		shell, executable := "sh", "touch"

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		args := []string{"-c", "$(" + executable + " " + testFile + ")"}

		cmd := cmdFunc(shell, args, nil)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %s", out, err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, rule.ID, "test_rule_pid1", "wrong rule triggered")

			if !rhel7 && !validateExecSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment || kind == dockerWrapperType {
				testContainerPath(t, event, "process.file.container_path")
				testStringFieldContains(t, event, "process.ancestors.file.container_path", "docker")
			}
		}
	})
}

func TestProcessExec(t *testing.T) {
	executable := "/usr/bin/touch"
	if resolved, err := os.Readlink(executable); err == nil {
		executable = resolved
	} else {
		if os.IsNotExist(err) {
			executable = "/bin/touch"
		}
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`exec.file.path == "%s"`, executable),
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	cmd := exec.Command("sh", "-c", executable+" /dev/null")
	if err := cmd.Run(); err != nil {
		t.Error(err)
	}

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		assertFieldEqual(t, event, "exec.file.path", executable)
		assertFieldOneOf(t, event, "process.file.name", []interface{}{"sh", "bash", "dash"})
		if testEnvironment == DockerEnvironment {
			testContainerPath(t, event, "exec.file.container_path")
		}
	}
}

func TestProcessMetadata(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_executable",
			Expression: `exec.file.path == "{{.Root}}/test-exec" && exec.file.uid == 98 && exec.file.gid == 99`,
		},
		{
			ID:         "test_metadata",
			Expression: `exec.file.path == "{{.Root}}/test-exec" && process.uid == 1001 && process.euid == 1002 && process.fsuid == 1002 && process.gid == 2001 && process.egid == 2002 && process.fsgid == 2002`,
		},
	}

	test, err := newTestModule(nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o777
	expectedMode := applyUmask(fileMode)
	testFile, _, err := test.CreateWithOptions("test-exec", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	f, err := os.OpenFile(testFile, os.O_WRONLY, 0)
	if err != nil {
		t.Error(err)
	}
	f.WriteString("#!/bin/bash\n")
	f.Close()

	t.Run("executable", func(t *testing.T) {
		cmd := exec.Command(testFile)
		if err := cmd.Run(); err != nil {
			t.Error(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "exec", "wrong event type")
			assertRights(t, event.Exec.FileFields.Mode, uint16(expectedMode))
			assertNearTime(t, event.Exec.FileFields.MTime)
			assertNearTime(t, event.Exec.FileFields.CTime)
		}
	})

	t.Run("credentials", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 2001, 2001, 0); errno != 0 {
				t.Error(errno)
			}
			if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 1001, 1001, 0); errno != 0 {
				t.Error(errno)
			}

			if _, err := syscall.ForkExec(testFile, []string{}, nil); err != nil {
				t.Error(err)
			}
		}()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "exec", "wrong event type")

			assert.Equal(t, int(event.Exec.Credentials.UID), 1001, "wrong uid")
			assert.Equal(t, int(event.Exec.Credentials.EUID), 1001, "wrong euid")
			assert.Equal(t, int(event.Exec.Credentials.FSUID), 1001, "wrong fsuid")
			assert.Equal(t, int(event.Exec.Credentials.GID), 2001, "wrong gid")
			assert.Equal(t, int(event.Exec.Credentials.EGID), 2001, "wrong egid")
			assert.Equal(t, int(event.Exec.Credentials.FSGID), 2001, "wrong fsgid")
		}
	})
}

func TestProcessLineage(t *testing.T) {
	executable := "/usr/bin/touch"
	if resolved, err := os.Readlink(executable); err == nil {
		executable = resolved
	} else {
		if os.IsNotExist(err) {
			executable = "/bin/touch"
		}
	}

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`exec.file.path == "%s" && exec.args in [~"*01010101*"]`, executable),
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	cmd := exec.Command(executable, "-t", "01010101", "/dev/null")
	if err := cmd.Run(); err != nil {
		t.Error(err)
	}

	t.Run("fork", func(t *testing.T) {
		event, err := test.GetProbeEvent(3*time.Second, "fork")
		if err != nil {
			t.Error(err)
		} else {
			testProcessLineageFork(t, event)
		}
	})

	var execPid int

	t.Run("exec", func(t *testing.T) {
		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if err := testProcessLineageExec(t, event); err != nil {
				t.Error(err)
			} else {
				execPid = int(event.ProcessContext.Pid)
			}
		}
	})

	t.Run("exit", func(t *testing.T) {
		timeout := time.After(3 * time.Second)
		var event *probe.Event

		for {
			select {
			case <-timeout:
				t.Error(errors.New("timeout"))
				return
			default:
				event, err = test.GetProbeEvent(3*time.Second, "exit")
				if err != nil {
					continue
				}

				if int(event.ProcessContext.Pid) == execPid {
					testProcessLineageExit(t, event, test)
					return
				}
			}
		}
	})
}

func testProcessLineageExec(t *testing.T, event *probe.Event) error {
	// check for the new process context
	cacheEntry := event.ResolveProcessCacheEntry()
	if cacheEntry == nil {
		return errors.New("expected a process cache entry, got nil")
	} else {
		// make sure the container ID was properly inherited from the parent
		if cacheEntry.Ancestor == nil {
			return errors.New("expected a parent, got nil")
		} else {
			assert.Equal(t, cacheEntry.ContainerID, cacheEntry.Ancestor.ContainerID)
		}
	}

	if testEnvironment == DockerEnvironment {
		testContainerPath(t, event, "process.file.container_path")
	}

	return nil
}

func testProcessLineageFork(t *testing.T, event *probe.Event) {
	// we need to make sure that the child entry if properly populated with its parent metadata
	newEntry := event.ResolveProcessCacheEntry()
	if newEntry == nil {
		t.Errorf("expected a new process cache entry, got nil")
	} else {
		// fetch the parent of the new entry, it should the test binary itself
		parentEntry := newEntry.Ancestor

		if parentEntry == nil {
			t.Errorf("expected a parent cache entry, got nil")
		} else {
			// checking cookie and pathname str should be enough to make sure that the metadata were properly
			// copied from kernel space (those 2 information are stored in 2 different maps)
			assert.Equal(t, newEntry.Cookie, parentEntry.Cookie, "wrong cookie")
			assert.Equal(t, newEntry.PPid, parentEntry.Pid, "wrong ppid")
			assert.Equal(t, newEntry.ContainerID, parentEntry.ContainerID, "wrong container id")

			// We can't check that the new entry is in the list of the children of its parent because the exit event
			// has probably already been processed (thus the parent list of children has already been updated and the
			// child entry deleted).
		}

		if testEnvironment == DockerEnvironment {
			testContainerPath(t, event, "process.file.container_path")
		}
	}
}

func testProcessLineageExit(t *testing.T, event *probe.Event, test *testModule) {
	// make sure that the process cache entry of the process was properly deleted from the cache
	err := retry.Do(func() error {
		resolvers := test.probe.GetResolvers()
		entry := resolvers.ProcessResolver.Get(event.ProcessContext.Pid)
		if entry != nil {
			return fmt.Errorf("the process cache entry was not deleted from the user space cache")
		}

		return nil
	})

	if err != nil {
		t.Error(err)
	}
}

func TestProcessCredentialsUpdate(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_setuid",
			Expression: `setuid.uid == 1001 && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_setreuid",
			Expression: `setuid.uid == 1002 && setuid.euid == 1003 && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_setfsuid",
			Expression: `setuid.fsuid == 1004 && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_setgid",
			Expression: `setgid.gid == 1005 && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_setregid",
			Expression: `setgid.gid == 1006 && setgid.egid == 1007 && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_setfsgid",
			Expression: `setgid.fsgid == 1008 && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_capset",
			Expression: `capset.cap_effective & CAP_WAKE_ALARM == 0 && capset.cap_permitted & CAP_SYS_BOOT == 0 && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("setuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETUID, 1001, 0, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setuid")
			assert.Equal(t, event.SetUID.UID, uint32(1001), "wrong uid")
		}
	})

	t.Run("setreuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 1002, 1003, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setreuid")
			assert.Equal(t, event.SetUID.UID, uint32(1002), "wrong uid")
			assert.Equal(t, event.SetUID.EUID, uint32(1003), "wrong euid")
		}
	})

	t.Run("setresuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETRESUID, 1002, 1003, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setreuid")
			assert.Equal(t, event.SetUID.UID, uint32(1002), "wrong uid")
			assert.Equal(t, event.SetUID.EUID, uint32(1003), "wrong euid")
		}
	})

	t.Run("setfsuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETFSUID, 1004, 0, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setfsuid")
			assert.Equal(t, event.SetUID.FSUID, uint32(1004), "wrong fsuid")
		}
	})

	t.Run("setgid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETGID, 1005, 0, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setgid")
			assert.Equal(t, event.SetGID.GID, uint32(1005), "wrong gid")
		}
	})

	t.Run("setregid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 1006, 1007, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setregid")
			assert.Equal(t, event.SetGID.GID, uint32(1006), "wrong gid")
			assert.Equal(t, event.SetGID.EGID, uint32(1007), "wrong egid")
		}
	})

	t.Run("setresgid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETRESGID, 1006, 1007, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setregid")
			assert.Equal(t, event.SetGID.GID, uint32(1006), "wrong gid")
			assert.Equal(t, event.SetGID.EGID, uint32(1007), "wrong egid")
		}
	})

	t.Run("setfsgid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETFSGID, 1008, 0, 0); errno != 0 {
				t.Error(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_setfsgid")
			assert.Equal(t, event.SetGID.FSGID, uint32(1008), "wrong gid")
		}
	})

	t.Run("capset", func(t *testing.T) {
		// Parse kernel capabilities of the current thread
		threadCapabilities, err := capability.NewPid2(0)
		if err != nil {
			t.Fatal(err)
		}
		if err := threadCapabilities.Load(); err != nil {
			t.Fatal(err)
		}

		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			// remove capabilities that we do not need
			threadCapabilities.Unset(capability.PERMITTED|capability.EFFECTIVE, capability.CAP_SYS_BOOT)
			threadCapabilities.Unset(capability.EFFECTIVE, capability.CAP_WAKE_ALARM)
			if err := threadCapabilities.Apply(capability.CAPS); err != nil {
				t.Error(err)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_capset")

			// transform the collected kernel capabilities into a usable capability set
			newSet, err := capability.NewPid2(0)
			if err != nil {
				t.Fatal(err)
			}
			newSet.Clear(capability.PERMITTED | capability.EFFECTIVE)
			parseCapIntoSet(event.Capset.CapEffective, capability.EFFECTIVE, newSet, t)
			parseCapIntoSet(event.Capset.CapPermitted, capability.PERMITTED, newSet, t)

			for _, c := range capability.List() {
				if expectedValue := threadCapabilities.Get(capability.EFFECTIVE, c); expectedValue != newSet.Get(capability.EFFECTIVE, c) {
					t.Errorf("expected incorrect %s flag in cap_effective, expected %v", c, expectedValue)
				}
				if expectedValue := threadCapabilities.Get(capability.PERMITTED, c); expectedValue != newSet.Get(capability.PERMITTED, c) {
					t.Errorf("expected incorrect %s flag in cap_permitted, expected %v", c, expectedValue)
				}
			}
		}
	})

	// test_capset can be somewhat noisy and some events may leak to the next tests (there is a short delay between the
	// reset of the maps and the reload of the rules, we can't move the reset after the reload either, otherwise the
	// ruleset_reload test won't work => we would have reset the channel that contains the reload event).
	// Load a fake new test module to empty the rules and properly cleanup the channels.
	_, _ = newTestModule(nil, nil, testOpts{})
}

func parseCapIntoSet(capabilities uint64, flag capability.CapType, c capability.Capabilities, t *testing.T) {
	for _, v := range model.KernelCapabilityConstants {
		if v == 0 {
			continue
		}

		if int(capabilities)&v == v {
			c.Set(flag, capability.Cap(math.Log2(float64(v))))
		}
	}
}
