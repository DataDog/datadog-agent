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

	"github.com/DataDog/gopsutil/process"
	"github.com/avast/retry-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/syndtr/gocapability/capability"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create("test-process")
		os.Remove(testFile)
		return err
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_rule")
	})
}

func TestProcessContext(t *testing.T) {
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		t.Fatalf("unable to find proc entry: %s", err)
	}

	filledProc := utils.GetFilledProcess(proc)
	if filledProc == nil {
		t.Fatal("unable to find proc entry")
	}
	execSince := time.Since(time.Unix(0, filledProc.CreateTime*int64(time.Millisecond)))
	waitUntil := execSince + getEventTimeout + time.Second

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_inode",
			Expression: `open.file.path == "{{.Root}}/test-process-context" && open.flags & O_CREAT != 0`,
		},
		{
			ID:         "test_exec_time",
			Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-exec-time" && process.created_at > %ds`, int(waitUntil.Seconds())),
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

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("exec-time", func(t *testing.T) {
		testFile, _, err := test.Path("test-exec-time")
		if err != nil {
			t.Fatal(err)
		}

		err = test.GetSignal(t, func() error {
			f, err := os.Create(testFile)
			if err != nil {
				return err
			}

			return f.Close()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			t.Errorf("got event: %s", event)
		})
		if err == nil {
			t.Error("shouldn't get an event")
		}

		defer os.Remove(testFile)

		// ensure to exceed the delay
		time.Sleep(2 * time.Second)

		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDONLY, 0)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_exec_time")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	t.Run("inode", func(t *testing.T) {
		testFile, _, err := test.Path("test-process-context")
		if err != nil {
			t.Fatal(err)
		}

		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}

		test.WaitSignal(t, func() error {
			f, err := os.Create(testFile)
			if err != nil {
				return err
			}

			return f.Close()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertFieldEqual(t, event, "process.file.path", executable)
			assert.Equal(t, getInode(t, executable), event.ResolveProcessCacheEntry().FileFields.Inode, "wrong inode")
		})
	})

	test.Run(t, "args-envs", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al", "--password", "secret", "--custom", "secret"}
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			_ = cmd.Run()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
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

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	t.Run("argv", func(t *testing.T) {
		lsExecutable := which("ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "-ll")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_argv")
		})
	})

	t.Run("args-flags", func(t *testing.T) {
		lsExecutable := which("ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "-ls", "--escape")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_args_flags")
		})
	})

	t.Run("args-options", func(t *testing.T) {
		lsExecutable := which("ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "--block-size", "123")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_args_options")
		})
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

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			_ = cmd.Run()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			args, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(args.(string), " ")
			assert.Equal(t, 2, len(argv), "incorrect number of args: %s", argv)
			assert.Equal(t, true, strings.HasSuffix(argv[1], "..."), "args not truncated")
		})

		// number of args overflow
		nArgs, args := 200, []string{"-al"}
		for i := 0; i != nArgs; i++ {
			args = append(args, "aaa")
		}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			_ = cmd.Run()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			args, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(args.(string), " ")
			n := len(argv)
			if n == 0 || n > nArgs {
				t.Errorf("incorrect number of args %d: %s", n, args.(string))
			}

			truncated, err := event.GetFieldValue("exec.args_truncated")
			if err != nil {
				t.Errorf("not able to get args truncated")
			}
			if !truncated.(bool) {
				t.Errorf("arg not truncated: %s", args.(string))
			}

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
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

		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
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

			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertFieldEqual(t, event, "process.file.path", executable)

			if name, _ := event.GetFieldValue("process.tty_name"); !strings.HasPrefix(name.(string), "pts") {
				t.Errorf("not able to get a tty name: %s\n", name)
			}

			if inode := getInode(t, executable); inode != event.ResolveProcessCacheEntry().FileFields.Inode {
				t.Errorf("expected inode %d, got %d => %+v", event.ResolveProcessCacheEntry().FileFields.Inode, inode, event)
			}

			str := event.String()

			if !strings.Contains(str, "pts") {
				t.Error("tty not serialized")
			}
		})
	})

	test.Run(t, "ancestors", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-process-ancestors")
		if err != nil {
			t.Fatal(err)
		}

		executable := "touch"

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		args := []string{"-c", "$(" + executable + " " + testFile + ")"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("sh", args, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s: %s", out, err)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_ancestors")
			assert.Equal(t, "sh", event.ProcessContext.Ancestor.Comm)

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	test.Run(t, "pid1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-process-pid1")
		if err != nil {
			t.Fatal(err)
		}

		shell, executable := "sh", "touch"

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		args := []string{"-c", "$(" + executable + " " + testFile + ")"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(shell, args, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s: %s", out, err)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_pid1", rule.ID, "wrong rule triggered")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	test.Run(t, "service-tag", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-process-context")
		if err != nil {
			t.Fatal(err)
		}

		shell, executable := "sh", "touch"

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		args := []string{"-c", "$(" + executable + " " + testFile + ")"}
		envs := []string{"DD_SERVICE=myservice"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(shell, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s: %s", out, err)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_inode", rule.ID, "wrong rule triggered")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}

			service := event.GetProcessServiceTag()
			assert.Equal(t, service, "myservice")
		})
	})
}

func TestProcessExecCTime(t *testing.T) {
	executable := "/usr/bin/touch"
	if resolved, err := os.Readlink(executable); err == nil {
		executable = resolved
	} else {
		if os.IsNotExist(err) {
			executable = "/bin/touch"
		}
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_exec_ctime",
		Expression: "exec.file.change_time < 5s",
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Path("touch")
		if err != nil {
			t.Fatal(err)
		}
		copyFile(executable, testFile, 0755)

		cmd := exec.Command(testFile, "/tmp/test")
		return cmd.Run()
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assert.Equal(t, "test_exec_ctime", rule.ID, "wrong rule triggered")

		if !validateExecSchema(t, event) {
			t.Error(event.String())
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

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		cmd := exec.Command("sh", "-c", executable+" /dev/null")
		return cmd.Run()
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assertFieldEqual(t, event, "exec.file.path", executable)
		assertFieldOneOf(t, event, "process.file.name", []interface{}{"sh", "bash", "dash"})
	})
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

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
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
		test.WaitSignal(t, func() error {
			cmd := exec.Command(testFile)
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "exec", event.GetType(), "wrong event type")
			assertRights(t, event.Exec.FileFields.Mode, uint16(expectedMode))
			assertNearTime(t, event.Exec.FileFields.MTime)
			assertNearTime(t, event.Exec.FileFields.CTime)
		})
	})

	t.Run("credentials", func(t *testing.T) {
		test.WaitSignal(t, func() error {
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
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "exec", event.GetType(), "wrong event type")

			assert.Equal(t, 1001, int(event.Exec.Credentials.UID), "wrong uid")
			assert.Equal(t, 1001, int(event.Exec.Credentials.EUID), "wrong euid")
			assert.Equal(t, 1001, int(event.Exec.Credentials.FSUID), "wrong fsuid")
			assert.Equal(t, 2001, int(event.Exec.Credentials.GID), "wrong gid")
			assert.Equal(t, 2001, int(event.Exec.Credentials.EGID), "wrong egid")
			assert.Equal(t, 2001, int(event.Exec.Credentials.FSGID), "wrong fsgid")
		})
	})
}

func TestProcessExecExit(t *testing.T) {
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

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var execPid int // will be set by the first fork event

	err = test.GetProbeEvent(func() error {
		cmd := exec.Command(executable, "-t", "01010101", "/dev/null")
		return cmd.Run()
	}, func(event *sprobe.Event) bool {
		switch event.GetEventType() {
		case model.ExecEventType:
			if testProcessEEIsExpectedExecEvent(event) {
				execPid = int(event.ProcessContext.Pid)
				if err := testProcessEEExec(t, event); err != nil {
					t.Error(err)
				}
			}
		case model.ExitEventType:
			if execPid != 0 && int(event.ProcessContext.Pid) == execPid {
				return true
			}
		}
		return false
	}, time.Second*3, model.ExecEventType, model.ExitEventType)
	if err != nil {
		t.Error(err)
	}

	testProcessEEExit(t, uint32(execPid), test)
}

func testProcessEEIsExpectedExecEvent(event *sprobe.Event) bool {
	if event.GetEventType() != model.ExecEventType {
		return false
	}

	basename, err := event.GetFieldValue("exec.file.name")
	if err != nil {
		return false
	}

	if basename.(string) != "touch" {
		return false
	}

	args, err := event.GetFieldValue("exec.args")
	if err != nil {
		return false
	}

	return strings.Contains(args.(string), "01010101")
}

func testProcessEEExec(t *testing.T, event *sprobe.Event) error {
	// check for the new process context
	cacheEntry := event.ResolveProcessCacheEntry()
	if cacheEntry == nil {
		return errors.New("expected a process cache entry, got nil")
	}
	// make sure the container ID was properly inherited from the parent
	if cacheEntry.Ancestor == nil {
		return errors.New("expected a parent, got nil")
	}
	assert.Equal(t, cacheEntry.Ancestor.ContainerID, cacheEntry.ContainerID)

	return nil
}

func testProcessEEExit(t *testing.T, pid uint32, test *testModule) {
	// make sure that the process cache entry of the process was properly deleted from the cache
	err := retry.Do(func() error {
		resolvers := test.probe.GetResolvers()
		entry := resolvers.ProcessResolver.Get(pid)
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

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("setuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()

				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETUID, 1001, 0, 0); errno != 0 {
					t.Error(errno)
				}
			}()

			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setuid")
			assert.Equal(t, uint32(1001), event.SetUID.UID, "wrong uid")
		})
	})

	t.Run("setreuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 1002, 1003, 0); errno != 0 {
					t.Error(errno)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setreuid")
			assert.Equal(t, uint32(1002), event.SetUID.UID, "wrong uid")
			assert.Equal(t, uint32(1003), event.SetUID.EUID, "wrong euid")
		})
	})

	t.Run("setresuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETRESUID, 1002, 1003, 0); errno != 0 {
					t.Error(errno)
				}
			}()

			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setreuid")
			assert.Equal(t, uint32(1002), event.SetUID.UID, "wrong uid")
			assert.Equal(t, uint32(1003), event.SetUID.EUID, "wrong euid")
		})
	})

	t.Run("setfsuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETFSUID, 1004, 0, 0); errno != 0 {
					t.Error(errno)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setfsuid")
			assert.Equal(t, uint32(1004), event.SetUID.FSUID, "wrong fsuid")
		})
	})

	t.Run("setgid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETGID, 1005, 0, 0); errno != 0 {
					t.Error(errno)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setgid")
			assert.Equal(t, uint32(1005), event.SetGID.GID, "wrong gid")
		})
	})

	t.Run("setregid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 1006, 1007, 0); errno != 0 {
					t.Error(errno)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setregid")
			assert.Equal(t, uint32(1006), event.SetGID.GID, "wrong gid")
			assert.Equal(t, uint32(1007), event.SetGID.EGID, "wrong egid")
		})
	})

	t.Run("setresgid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETRESGID, 1006, 1007, 0); errno != 0 {
					t.Error(errno)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setregid")
			assert.Equal(t, uint32(1006), event.SetGID.GID, "wrong gid")
			assert.Equal(t, uint32(1007), event.SetGID.EGID, "wrong egid")
		})
	})

	t.Run("setfsgid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				if _, _, errno := syscall.Syscall(syscall.SYS_SETFSGID, 1008, 0, 0); errno != 0 {
					t.Error(errno)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setfsgid")
			assert.Equal(t, uint32(1008), event.SetGID.FSGID, "wrong gid")
		})
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

		var threadCapabilitiesLock sync.Mutex

		test.WaitSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()
				threadCapabilitiesLock.Lock()
				defer threadCapabilitiesLock.Unlock()

				runtime.LockOSThread()
				// do not unlock, we want the thread to be killed when exiting the goroutine

				// remove capabilities that we do not need
				threadCapabilities.Unset(capability.PERMITTED|capability.EFFECTIVE, capability.CAP_SYS_BOOT)
				threadCapabilities.Unset(capability.EFFECTIVE, capability.CAP_WAKE_ALARM)
				if err := threadCapabilities.Apply(capability.CAPS); err != nil {
					t.Error(err)
				}
			}()
			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_capset")

			// transform the collected kernel capabilities into a usable capability set
			newSet, err := capability.NewPid2(0)
			if err != nil {
				t.Error(err)
			}
			newSet.Clear(capability.PERMITTED | capability.EFFECTIVE)
			parseCapIntoSet(event.Capset.CapEffective, capability.EFFECTIVE, newSet, t)
			parseCapIntoSet(event.Capset.CapPermitted, capability.PERMITTED, newSet, t)

			threadCapabilitiesLock.Lock()
			defer threadCapabilitiesLock.Unlock()

			for _, c := range capability.List() {
				if expectedValue := threadCapabilities.Get(capability.EFFECTIVE, c); expectedValue != newSet.Get(capability.EFFECTIVE, c) {
					t.Errorf("expected incorrect %s flag in cap_effective, expected %v", c, expectedValue)
				}
				if expectedValue := threadCapabilities.Get(capability.PERMITTED, c); expectedValue != newSet.Get(capability.PERMITTED, c) {
					t.Errorf("expected incorrect %s flag in cap_permitted, expected %v", c, expectedValue)
				}
			}
		})
	})

	// test_capset can be somewhat noisy and some events may leak to the next tests (there is a short delay between the
	// reset of the maps and the reload of the rules, we can't move the reset after the reload either, otherwise the
	// ruleset_reload test won't work => we would have reset the channel that contains the reload event).
	// Load a fake new test module to empty the rules and properly cleanup the channels.
	_, _ = newTestModule(t, nil, nil, testOpts{})
}

func parseCapIntoSet(capabilities uint64, flag capability.CapType, c capability.Capabilities, t *testing.T) {
	for _, v := range model.KernelCapabilityConstants {
		if v == 0 {
			continue
		}

		if capabilities&v == v {
			c.Set(flag, capability.Cap(math.Log2(float64(v))))
		}
	}
}
