// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
	"github.com/syndtr/gocapability/capability"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestProcess(t *testing.T) {
	SkipIfNotAvailable(t)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDef := &rules.RuleDefinition{
		ID: "test_rule",
	}

	if ebpfLessEnabled {
		ruleDef.Expression = fmt.Sprintf(`process.file.name == "%s" && open.file.path == "{{.Root}}/test-process"`, path.Base(executable))
	} else {
		ruleDef.Expression = fmt.Sprintf(`process.user != "" && process.file.name == "%s" && open.file.path == "{{.Root}}/test-process"`, path.Base(executable))
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create("test-process")
		if err != nil {
			return err
		}
		return os.Remove(testFile)
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_rule")
	})
}

func TestProcessEBPFLess(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		t.Skip("ebpfless specific")
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFLessProbe)
	if !ok {
		t.Skip("not supported")
	}

	t.Run("proc-scan", func(t *testing.T) {
		err := retry.Do(func() error {
			var found bool
			p.Resolvers.ProcessResolver.Walk(func(entry *model.ProcessCacheEntry) {
				if entry.FileEvent.BasenameStr == path.Base(executable) && slices.Contains(entry.ArgsEntry.Values, "-trace") {
					found = true
				}
			})

			if !found {
				return errors.New("not found")
			}
			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10))
		assert.NoError(t, err)
	})
}

func TestProcessContext(t *testing.T) {
	SkipIfNotAvailable(t)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_inode",
			Expression: `open.file.path == "{{.Root}}/test-process-context" && open.flags & O_CREAT != 0`,
		},
		{
			ID:         "test_exec_time_1",
			Expression: `open.file.path == "{{.Root}}/test-exec-time-1" && process.created_at == 0`,
		},
		{
			ID:         "test_exec_time_2",
			Expression: `open.file.path == "{{.Root}}/test-exec-time-2" && process.created_at > 1s`,
		},
		{
			ID:         "test_rule_ancestors",
			Expression: `open.file.path == "{{.Root}}/test-process-ancestors" && process.ancestors[A].file.name in ["sh", "dash", "bash"]`,
		},
		{
			ID:         "test_rule_parent",
			Expression: `open.file.path == "{{.Root}}/test-process-parent" && process.parent.file.name in ["sh", "dash", "bash"]`,
		},
		{
			ID:         "test_rule_pid1",
			Expression: `open.file.path == "{{.Root}}/test-process-pid1" && process.ancestors[A].pid == 1`,
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
		{
			ID:         "test_rule_ancestors_args",
			Expression: `open.file.path == "{{.Root}}/test-ancestors-args" && process.ancestors.args_flags == "c" && process.ancestors.args_flags == "x"`,
		},
		{
			ID:         "test_rule_envp",
			Expression: `exec.file.name == "ls" && exec.envp in ["ENVP=test"] && exec.args =~ "*example.com"`,
		},
		{
			ID:         "test_rule_args_envs_dedup",
			Expression: `exec.file.name == "ls" && exec.argv == "test123456"`,
		},
		{
			ID:         "test_rule_ancestors_glob",
			Expression: `exec.file.name == "ls" && exec.argv == "glob" && process.ancestors.file.path in [~"/tmp/**"]`,
		},
		{
			ID:         "test_self_exec",
			Expression: `exec.file.name in ["syscall_tester", "exe"] && exec.argv0 == "selfexec123" && process.comm == "exe"`,
		},
		{
			ID:         "test_rule_ctx_1",
			Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-process-ctx-1" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_rule_ctx_2",
			Expression: `open.file.path == "{{.Root}}/test-process-ctx-2" && process.file.name != ""`,
		},
		{
			ID:         "test_rule_ctx_3",
			Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-process-ctx-3" && process.file.path == "%s"`, executable),
		},
		{
			ID:         "test_rule_ctx_4",
			Expression: `open.file.path == "{{.Root}}/test-process-ctx-4" && process.file.path != ""`,
		},
		{
			ID:         "test_rule_container",
			Expression: `exec.file.name == "touch" && exec.argv == "{{.Root}}/test-container"`,
		},
		{
			ID:         "test_event_service",
			Expression: `open.file.path == "{{.Root}}/test-event-service" && open.flags & O_CREAT != 0 && event.service == "myservice"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("exec-time", func(t *testing.T) {
		testFile, _, err := test.Path("test-exec-time-1")
		if err != nil {
			t.Fatal(err)
		}

		err = test.GetSignal(t, func() error {
			f, err := os.Create(testFile)
			if err != nil {
				return err
			}
			f.Close()
			os.Remove(testFile)

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			t.Errorf("shouldn't get an event: got event: %s", test.debugEvent(event))
		})
		if err == nil {
			t.Fatal("shouldn't get an event")
		}

		test.WaitSignal(t, func() error {
			testFile, _, err := test.Path("test-exec-time-2")
			if err != nil {
				t.Fatal(err)
			}

			f, err := os.Create(testFile)
			if err != nil {
				return err
			}
			f.Close()
			os.Remove(testFile)

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_exec_time_2")
		})
	})

	t.Run("inode", func(t *testing.T) {
		SkipIfNotAvailable(t)

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
		}, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqual(t, event, "process.file.path", executable)

			assert.Equal(t, getInode(t, executable), event.ProcessContext.FileEvent.Inode, "wrong inode")
		})
	})

	test.Run(t, "args-envs", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al", "--password", "secret", "--custom", "secret"}
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib", "DD_API_KEY=dd-api-key"}
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			// we need to ignore the error because "--password" is not a valid option for ls
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			argv0, err := event.GetFieldValue("exec.argv0")
			if err != nil {
				t.Errorf("not able to get argv0")
			}
			assert.Equal(t, "ls", argv0, "incorrect argv0: %s", argv0)

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
					if strings.Contains(env, s) {
						return true
					}
				}
				return false
			}

			if !contains("LD_LIBRARY_PATH") {
				t.Errorf("env not found: %v", event)
			}

			// trigger serialization to test scrubber
			str, err := test.marshalEvent(event)
			if err != nil {
				t.Error(err)
			}

			if !strings.Contains(str, "password") || !strings.Contains(str, "custom") {
				t.Error("args not serialized")
			}

			if strings.Contains(str, "secret") || strings.Contains(str, "dd-api-key") {
				t.Error("secret or env values exposed")
			}
		}))
	})

	test.Run(t, "envp", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al", "http://example.com"}
		envs := []string{"ENVP=test"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_envp", rule.ID, "wrong rule triggered")
		}))
	})

	t.Run("argv", func(t *testing.T) {
		lsExecutable := which(t, "ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "-ll")
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_argv")
		}))
	})

	t.Run("args-flags", func(t *testing.T) {
		lsExecutable := which(t, "ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "-ls", "--escape")
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_args_flags")
		}))
	})

	t.Run("args-options", func(t *testing.T) {
		lsExecutable := which(t, "ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "--block-size", "123")
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_args_options")
		}))
	})

	test.Run(t, "args-overflow-single", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
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
			// we need to ignore the error because the string of "a" generates a "File name too long" error
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			args, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(args.(string), " ")
			assert.Equal(t, 2, len(argv), "incorrect number of args: %s", argv)
			assert.Equal(t, model.MaxArgEnvSize-1, len(argv[1]), "wrong arg length")
			assert.Equal(t, true, strings.HasSuffix(argv[1], "..."), "args not truncated")

			// truncated is reported if a single argument is truncated or if the list is truncated
			truncated, err := event.GetFieldValue("exec.args_truncated")
			if err != nil {
				t.Errorf("not able to get args truncated")
			}
			if !truncated.(bool) {
				t.Errorf("arg not truncated: %s", args.(string))
			}

			argv0, err := event.GetFieldValue("exec.argv0")
			if err != nil {
				t.Errorf("not able to get argv0")
			}
			assert.Equal(t, "ls", argv0, "incorrect argv0: %s", argv0)
		}))
	})

	test.Run(t, "args-overflow-list-50", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}

		// force seed to have something we can reproduce
		rand.Seed(1)

		// number of args overflow
		nArgs, args := 1024, []string{"-al"}
		for i := 0; i != nArgs; i++ {
			args = append(args, utils.RandString(50))
		}

		err := test.GetSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			// we need to ignore the error because the string of "a" generates a "File name too long" error
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			execArgs, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(execArgs.(string), " ")
			if ebpfLessEnabled {
				assert.Equal(t, model.MaxArgsEnvsSize-1, len(argv), "incorrect number of args: %s", argv)
				for i := 0; i != model.MaxArgsEnvsSize-1; i++ {
					assert.Equal(t, args[i], argv[i], "expected arg not found")
				}
			} else {
				assert.Equal(t, 439, len(argv), "incorrect number of args: %s", argv)
				for i := 0; i != 439; i++ {
					assert.Equal(t, args[i], argv[i], "expected arg not found")
				}
			}

			// truncated is reported if a single argument is truncated or if the list is truncated
			truncated, err := event.GetFieldValue("exec.args_truncated")
			if err != nil {
				t.Errorf("not able to get args truncated")
			}
			if !truncated.(bool) {
				t.Errorf("arg not truncated: %s", execArgs.(string))
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	test.Run(t, "args-overflow-list-500", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}

		// force seed to have something we can reproduce
		rand.Seed(1)

		// number of args overflow
		nArgs, args := 1024, []string{"-al"}
		for i := 0; i != nArgs; i++ {
			args = append(args, utils.RandString(500))
		}

		err := test.GetSignal(t, func() error {
			cmd := cmdFunc("ls", args, envs)
			// we need to ignore the error because the string of "a" generates a "File name too long" error
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			execArgs, err := event.GetFieldValue("exec.args")
			if err != nil {
				t.Errorf("not able to get args")
			}

			argv := strings.Split(execArgs.(string), " ")
			if ebpfLessEnabled {
				assert.Equal(t, model.MaxArgsEnvsSize-1, len(argv), "incorrect number of args: %s", argv)
				for i := 0; i != model.MaxArgsEnvsSize-1; i++ {
					expected := args[i]
					if len(expected) > model.MaxArgEnvSize {
						expected = args[i][:model.MaxArgEnvSize-4] + "..." // 4 is the size number of the string
					}
					assert.Equal(t, expected, argv[i], "expected arg not found")
				}
			} else {
				assert.Equal(t, 457, len(argv), "incorrect number of args: %s", argv)
				for i := 0; i != 457; i++ {
					expected := args[i]
					if len(expected) > model.MaxArgEnvSize {
						expected = args[i][:model.MaxArgEnvSize-4] + "..." // 4 is the size number of the string
					}
					assert.Equal(t, expected, argv[i], "expected arg not found")
				}
			}

			// truncated is reported if a single argument is truncated or if the list is truncated
			truncated, err := event.GetFieldValue("exec.args_truncated")
			if err != nil {
				t.Errorf("not able to get args truncated")
			}
			if !truncated.(bool) {
				t.Errorf("arg not truncated: %s", execArgs.(string))
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	test.Run(t, "envs-overflow-single", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al"}
		envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}

		// size overflow
		var long string
		for i := 0; i != 1024; i++ {
			long += "a"
		}
		long += "="
		envs = append(envs, long)

		if kind == dockerWrapperType {
			args = []string{"-u", "PATH", "-u", "HOSTNAME", "-u", "HOME", "ls", "-al"}
		}

		test.WaitSignal(t, func() error {
			bin := "ls"
			if kind == dockerWrapperType {
				bin = "env"
			}
			cmd := cmdFunc(bin, args, envs)
			return cmd.Run()
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			execEnvp, err := event.GetFieldValue("exec.envp")
			if err != nil {
				t.Errorf("not able to get exec.envp")
			}

			envp := (execEnvp.([]string))
			assert.Equal(t, 2, len(envp), "incorrect number of envs: %s", envp)
			assert.Equal(t, model.MaxArgEnvSize-1, len(envp[1]), "wrong env length")
			assert.Equal(t, true, strings.HasSuffix(envp[1], "..."), "envs not truncated")

			// truncated is reported if a single environment variable is truncated or if the list is truncated
			truncated, err := event.GetFieldValue("exec.envs_truncated")
			if err != nil {
				t.Errorf("not able to get envs truncated")
			}
			if !truncated.(bool) {
				t.Errorf("envs not truncated: %s", execEnvp.([]string))
			}

			assert.Equal(t, envs[0], envp[0], "expected first env variable")
		}))
	})

	test.Run(t, "envs-overflow-list-50", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al"}

		// force seed to have something we can reproduce
		rand.Seed(1)

		// number of envs overflow
		nEnvs, envs := 1024, []string{"LD_LIBRARY_PATH=/tmp/lib"}
		var buf bytes.Buffer
		buf.Grow(50)
		for i := 0; i != nEnvs; i++ {
			buf.Reset()
			fmt.Fprintf(&buf, "%s=%s", utils.RandString(19), utils.RandString(30))
			envs = append(envs, buf.String())
		}

		if kind == dockerWrapperType {
			args = []string{"-u", "PATH", "-u", "HOSTNAME", "-u", "HOME", "ls", "-al"}
		}

		err := test.GetSignal(t, func() error {
			bin := "ls"
			if kind == dockerWrapperType {
				bin = "env"
			}
			cmd := cmdFunc(bin, args, envs)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			execEnvp, err := event.GetFieldValue("exec.envp")
			if err != nil {
				t.Errorf("not able to get exec.envp")
			}

			envp := (execEnvp.([]string))
			if ebpfLessEnabled {
				assert.Equal(t, model.MaxArgsEnvsSize, len(envp), "incorrect number of envs: %s", envp)
				for i := 0; i != model.MaxArgsEnvsSize; i++ {
					assert.Equal(t, envs[i], envp[i], "expected env not found")
				}
			} else {
				assert.Equal(t, 704, len(envp), "incorrect number of envs: %s", envp)
				for i := 0; i != 704; i++ {
					assert.Equal(t, envs[i], envp[i], "expected env not found")
				}
			}

			// truncated is reported if a single environment variable is truncated or if the list is truncated
			truncated, err := event.GetFieldValue("exec.envs_truncated")
			if err != nil {
				t.Errorf("not able to get envs truncated")
			}
			if !truncated.(bool) {
				t.Errorf("envs not truncated: %s", execEnvp.([]string))
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	test.Run(t, "envs-overflow-list-500", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"-al"}

		// force seed to have something we can reproduce
		rand.Seed(1)

		// number of envs overflow
		nEnvs, envs := 1024, []string{"LD_LIBRARY_PATH=/tmp/lib"}
		var buf bytes.Buffer
		buf.Grow(500)
		for i := 0; i != nEnvs; i++ {
			buf.Reset()
			fmt.Fprintf(&buf, "%s=%s", utils.RandString(199), utils.RandString(300))
			envs = append(envs, buf.String())
		}

		if kind == dockerWrapperType {
			args = []string{"-u", "PATH", "-u", "HOSTNAME", "-u", "HOME", "ls", "-al"}
		}

		err := test.GetSignal(t, func() error {
			bin := "ls"
			if kind == dockerWrapperType {
				bin = "env"
			}
			cmd := cmdFunc(bin, args, envs)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_args_envs")

			execEnvp, err := event.GetFieldValue("exec.envp")
			if err != nil {
				t.Errorf("not able to get exec.envp")
			}

			envp := (execEnvp.([]string))
			if ebpfLessEnabled {
				assert.Equal(t, model.MaxArgsEnvsSize, len(envp), "incorrect number of envs: %s", envp)
				for i := 0; i != model.MaxArgsEnvsSize; i++ {
					expected := envs[i]
					if len(expected) > model.MaxArgEnvSize {
						expected = envs[i][:model.MaxArgEnvSize-4] + "..." // 4 is the size number of the string
					}
					assert.Equal(t, expected, envp[i], "expected env not found")
				}
			} else {
				assert.Equal(t, 863, len(envp), "incorrect number of envs: %s", envp)
				for i := 0; i != 863; i++ {
					expected := envs[i]
					if len(expected) > model.MaxArgEnvSize {
						expected = envs[i][:model.MaxArgEnvSize-4] + "..." // 4 is the size number of the string
					}
					assert.Equal(t, expected, envp[i], "expected env not found")
				}
			}

			// truncated is reported if a single environment variable is truncated or if the list is truncated
			truncated, err := event.GetFieldValue("exec.envs_truncated")
			if err != nil {
				t.Errorf("not able to get envs truncated")
			}
			if !truncated.(bool) {
				t.Errorf("envs not truncated: %s", execEnvp.([]string))
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("args-envs-empty-strings", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"-al", ""}
			envs := []string{"LD_LIBRARY_PATH=/tmp/lib"}
			cmd := exec.Command("ls", args...)
			cmd.Env = envs
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_args_envs")

			args, err := event.GetFieldValue("exec.args")
			if err != nil || len(args.(string)) == 0 {
				t.Error("not able to get args")
			}
			assert.Contains(t, args.(string), "-al", "arg not found")

			// envs
			envs, err := event.GetFieldValue("exec.envs")
			if err != nil || len(envs.([]string)) == 0 {
				t.Error("not able to get envs")
			}

			contains := func(s string) bool {
				for _, env := range envs.([]string) {
					if strings.Contains(env, s) {
						return true
					}
				}
				return false
			}
			assert.True(t, contains("LD_LIBRARY_PATH"), "env not found")

			assert.False(t, event.Exec.ArgsTruncated, "args should not be truncated")
			assert.False(t, event.Exec.EnvsTruncated, "envs should not be truncated")
		}))
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

		test.WaitSignal(t, func() error {
			cmd := exec.Command("script", "/dev/null", "-c", fmt.Sprintf("%s slow-cat 4 %s", syscallTester, testFile))
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_tty")
			assertFieldEqual(t, event, "process.file.path", syscallTester)

			if name, _ := event.GetFieldValue("process.tty_name"); !strings.HasPrefix(name.(string), "pts") {
				t.Errorf("not able to get a tty name: %s\n", name)
			}

			assertInode(t, event.ProcessContext.FileEvent.Inode, getInode(t, syscallTester))

			str, err := test.marshalEvent(event)
			if err != nil {
				t.Error(err)
			}

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
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_ancestors")
			assert.Equal(t, "sh", event.ProcessContext.Ancestor.Comm)
		})
	})

	test.Run(t, "parent", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-process-parent")
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
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_parent")
			assert.Equal(t, "sh", event.ProcessContext.Parent.Comm)
			assert.Equal(t, "sh", event.ProcessContext.Ancestor.Comm)
		})
	})

	test.Run(t, "pid1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		SkipIfNotAvailable(t)

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
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_pid1", rule.ID, "wrong rule triggered")
		})
	})

	test.Run(t, "service-tag", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-event-service")
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
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_event_service", rule.ID, "wrong rule triggered")

			service := event.GetEventService()
			assert.Equal(t, service, "myservice")
		})
	})

	test.Run(t, "ancestors-args", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-ancestors-args")
		if err != nil {
			t.Fatal(err)
		}

		shell, executable := "sh", "touch"
		args := []string{"-x", "-c", "$(" + executable + " " + testFile + ")"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(shell, args, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_ancestors_args", rule.ID, "wrong rule triggered")
		})
	})

	test.Run(t, "args-envs-dedup", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		shell, args, envs := "sh", []string{"-x", "-c", "ls -al test123456; echo"}, []string{"DEDUP=dedup123"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(shell, args, envs)
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_args_envs_dedup", rule.ID, "wrong rule triggered")

			var data interface{}
			serialized, err := test.marshalEvent(event)
			if err != nil {
				t.Error(err)
			}

			if err := json.Unmarshal([]byte(serialized), &data); err != nil {
				t.Error(err)
			}

			if json, err := jsonpath.JsonPathLookup(data, "$.process.ancestors[0].args"); err == nil {
				t.Errorf("shouldn't have args, got %+v %s", json, serialized)
			}

			if json, err := jsonpath.JsonPathLookup(data, "$.process.ancestors[0].envs"); err == nil {
				t.Errorf("shouldn't have envs, got %+v : %s", json, serialized)
			}

			if json, err := jsonpath.JsonPathLookup(data, "$.process.ancestors[1].args"); err != nil {
				t.Errorf("should have args, got %+v %s", json, serialized)
			}

			if json, err := jsonpath.JsonPathLookup(data, "$.process.ancestors[1].envs"); err != nil {
				t.Errorf("should have envs, got %+v %s", json, serialized)
			}
		}))
	})

	t.Run("ancestors-glob", func(t *testing.T) {
		lsExecutable := which(t, "ls")

		test.WaitSignal(t, func() error {
			args := []string{"exec-in-pthread", lsExecutable, "glob"}
			cmd := exec.Command(syscallTester, args...)
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_ancestors_glob")
		}))
	})

	test.Run(t, "self-exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"self-exec", "selfexec123", "abc"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()

			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_self_exec")
		}))
	})

	test.Run(t, "container-id", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-container")
		if err != nil {
			t.Fatal(err)
		}

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		args := []string{"-c", "$(touch " + testFile + ")"}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("sh", args, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_rule_container", rule.ID, "wrong rule triggered")

			if kind == dockerWrapperType {
				assert.Equal(t, event.Exec.Process.ContainerID, event.ProcessContext.ContainerID)
				assert.Equal(t, event.Exec.Process.ContainerID, event.ProcessContext.Ancestor.ContainerID)
				assert.Equal(t, event.Exec.Process.ContainerID, event.ProcessContext.Parent.ContainerID)
			}
		}))
	})

	testProcessContextRule := func(t *testing.T, ruleID, filename string) {
		test.Run(t, ruleID, func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path(filename)
			if err != nil {
				t.Fatal(err)
			}

			test.WaitSignal(t, func() error {
				f, err := os.Create(testFile)
				if err != nil {
					return err
				}
				f.Close()
				return os.Remove(testFile)
			}, func(event *model.Event, rule *rules.Rule) {
				assert.Equal(t, ruleID, rule.ID, "wrong rule triggered")
			})
		})
	}

	testProcessContextRule(t, "test_rule_ctx_1", "test-process-ctx-1")
	testProcessContextRule(t, "test_rule_ctx_2", "test-process-ctx-2")
	testProcessContextRule(t, "test_rule_ctx_3", "test-process-ctx-3")
	testProcessContextRule(t, "test_rule_ctx_4", "test-process-ctx-4")
}

func TestProcessEnvsWithValue(t *testing.T) {
	SkipIfNotAvailable(t)

	lsExec := which(t, "ls")
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ldpreload_from_tmp_with_envs",
			Expression: fmt.Sprintf(`exec.file.path == "%s" && exec.envs in [~"LD_PRELOAD=*/tmp/*"]`, lsExec),
		},
	}

	opts := testOpts{
		envsWithValue: []string{"LD_PRELOAD"},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("ldpreload", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{}
			envp := []string{"LD_PRELOAD=/tmp/dyn.so"}

			cmd := exec.Command(lsExec, args...)
			cmd.Env = envp
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_ldpreload_from_tmp_with_envs")
			assertFieldEqual(t, event, "exec.file.path", lsExec)
			assertFieldStringArrayIndexedOneOf(t, event, "exec.envs", 0, []string{"LD_PRELOAD=/tmp/dyn.so"})
		}))
	})
}

func TestProcessExecCTime(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	ruleDef := &rules.RuleDefinition{
		ID:         "test_exec_ctime",
		Expression: "exec.file.change_time < 30s",
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Path("touch")
		if err != nil {
			return err
		}
		copyFile(executable, testFile, 0755)

		cmd := exec.Command(testFile, "/tmp/test")
		return cmd.Run()
	}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_exec_ctime", rule.ID, "wrong rule triggered")
	}))
}

func TestProcessPIDVariable(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule_var",
		Expression: `open.file.path =~ "/proc/*/maps" && open.file.path != "/proc/${process.pid}/maps"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		cmd := exec.Command(executable, fmt.Sprintf("/proc/%d/maps", os.Getpid()))
		return cmd.Run()
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_var", rule.ID, "wrong rule triggered")
	})
}

func TestProcessScopedVariable(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule_set_mutable_vars",
		Expression: `open.file.path == "{{.Root}}/test-open"`,
		Actions: []*rules.ActionDefinition{{
			Set: &rules.SetDefinition{
				Name:  "var1",
				Value: true,
				Scope: "process",
			},
		}, {
			Set: &rules.SetDefinition{
				Name:  "var2",
				Value: "disabled",
				Scope: "process",
			},
		}, {
			Set: &rules.SetDefinition{
				Name:  "var3",
				Value: []string{"aaa"},
				Scope: "process",
			},
		}, {
			Set: &rules.SetDefinition{
				Name:  "var3",
				Value: []string{"aaa"},
				Scope: "process",
			},
		}, {
			Set: &rules.SetDefinition{
				Name:  "var4",
				Field: "process.file.name",
			},
		}, {
			Set: &rules.SetDefinition{
				Name:  "var5",
				Field: "open.file.path",
			},
		}},
	}, {
		ID:         "test_rule_modify_mutable_vars",
		Expression: `open.file.path == "{{.Root}}/test-open-2"`,
		Actions: []*rules.ActionDefinition{{
			Set: &rules.SetDefinition{
				Name:  "var2",
				Value: "enabled",
				Scope: "process",
			},
		}, {
			Set: &rules.SetDefinition{
				Name:   "var3",
				Value:  []string{"bbb"},
				Scope:  "process",
				Append: true,
			},
		}},
	}, {
		ID: "test_rule_test_mutable_vars",
		Expression: `open.file.path == "{{.Root}}/test-open-3"` +
			`&& ${process.var1} == true` +
			`&& ${process.var2} == "enabled"` +
			`&& "aaa" in ${process.var3}` +
			`&& "bbb" in ${process.var3}` +
			`&& process.file.name == "${var4}"` +
			`&& open.file.path == "${var5}-3"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename1, filename2, filename3 string

	test.WaitSignal(t, func() error {
		filename1, _, err = test.Create("test-open")
		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_set_mutable_vars", rule.ID, "wrong rule triggered")
	})
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename1)

	test.WaitSignal(t, func() error {
		filename2, _, err = test.Create("test-open-2")
		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_modify_mutable_vars", rule.ID, "wrong rule triggered")
	})
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename2)

	test.WaitSignal(t, func() error {
		filename3, _, err = test.Create("test-open-3")
		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_test_mutable_vars", rule.ID, "wrong rule triggered")
	})
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename3)
}

func TestTimestampVariable(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule_set_timestamp_var",
		Expression: `open.file.path == "{{.Root}}/test-open"`,
		Actions: []*rules.ActionDefinition{{
			Set: &rules.SetDefinition{
				Name:  "timestamp1",
				Field: "event.timestamp",
				Scope: "process",
			},
		}},
	}, {
		ID:         "test_rule_test_timestamp_var",
		Expression: `open.file.path == "{{.Root}}/test-open-2" && ${process.timestamp1} > 0s && ${process.timestamp1} < 3s`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename1, filename2 string

	test.WaitSignal(t, func() error {
		filename1, _, err = test.Create("test-open")
		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_set_timestamp_var", rule.ID, "wrong rule triggered")
	})
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename1)

	test.WaitSignal(t, func() error {
		filename2, _, err = test.Create("test-open-2")
		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_test_timestamp_var", rule.ID, "wrong rule triggered")
	})
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename2)
}

func TestProcessExec(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`exec.file.path == "%s" && exec.args == "/dev/null"`, executable),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("exec", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("sh", "-c", executable+" /dev/null")
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqual(t, event, "exec.file.path", executable)
			assertFieldIsOneOf(t, event, "process.parent.file.name", []string{"sh", "bash", "dash"}, "wrong process parent file name")
			assertFieldStringArrayIndexedOneOf(t, event, "process.ancestors.file.name", 0, []string{"sh", "bash", "dash"})

			validateSyscallContext(t, event, "$.syscall.exec.path")
		}))
	})

	t.Run("exec-in-pthread", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"exec-in-pthread", executable, "/dev/null"}
			cmd := exec.Command(syscallTester, args...)
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqual(t, event, "exec.file.path", executable)
			assertFieldEqual(t, event, "process.parent.file.name", "syscall_tester", "wrong process parent file name")
		}))
	})
}

func TestProcessMetadata(t *testing.T) {
	SkipIfNotAvailable(t)

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

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := uint16(0o777)
	testFile, _, err := test.CreateWithOptions("test-exec", 98, 99, int(fileMode))
	if err != nil {
		t.Fatal(err)
	}

	if err = os.Chmod(testFile, os.FileMode(fileMode)); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(testFile, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("#!/bin/bash\n")
	f.Close()

	t.Run("executable", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command(testFile)
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "exec", event.GetType(), "wrong event type")
			assertRights(t, event.Exec.FileEvent.Mode, fileMode)
			if !ebpfLessEnabled {
				assertNearTime(t, event.Exec.FileEvent.MTime)
				assertNearTime(t, event.Exec.FileEvent.CTime)
			}
		}))
	})

	t.Run("credentials", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			attr := &syscall.SysProcAttr{
				Credential: &syscall.Credential{
					Uid: 1001,
					Gid: 2001,
				},
			}

			cmd := exec.Command(testFile)
			cmd.SysProcAttr = attr
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "exec", event.GetType(), "wrong event type")
			assert.Equal(t, 1001, int(event.Exec.Credentials.UID), "wrong uid")
			assert.Equal(t, 2001, int(event.Exec.Credentials.GID), "wrong gid")
			if !ebpfLessEnabled {
				assert.Equal(t, 1001, int(event.Exec.Credentials.EUID), "wrong euid")
				assert.Equal(t, 1001, int(event.Exec.Credentials.FSUID), "wrong fsuid")
				assert.Equal(t, 2001, int(event.Exec.Credentials.EGID), "wrong egid")
				assert.Equal(t, 2001, int(event.Exec.Credentials.FSGID), "wrong fsgid")
			}
		}))
	})
}

func TestProcessExecExit(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`exec.file.path == "%s" && exec.args in [~"*01010101*"]`, executable),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var execPid uint32
	var nsID uint64

	err = test.GetProbeEvent(func() error {
		cmd := exec.Command(executable, "-t", "01010101", "/dev/null")
		return cmd.Run()
	}, func(event *model.Event) bool {
		switch event.GetEventType() {
		case model.ExecEventType:
			if basename, err := event.GetFieldValue("exec.file.name"); err != nil || basename.(string) != "touch" {
				return false
			}
			if args, err := event.GetFieldValue("exec.args"); err != nil || !strings.Contains(args.(string), "01010101") {
				return false
			}

			validate := test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
				validateProcessContextLineage(t, event)
				validateProcessContextSECL(t, event)

				assertFieldEqual(t, event, "exec.file.name", "touch")
				assertFieldContains(t, event, "exec.args", "01010101")
			})
			validate(event, nil)

			execPid = event.ProcessContext.Pid
			if ebpfLessEnabled {
				nsID = event.ProcessContext.NSID
			}

		case model.ExitEventType:
			// assert that exit time >= exec time
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
			if execPid != 0 && event.ProcessContext.Pid == execPid {
				return true
			}
		}
		return false
	}, 3*time.Second, model.ExecEventType, model.ExitEventType)
	if err != nil {
		t.Error(err)
	}

	assert.NotEmpty(t, execPid, "exec pid not found")

	// make sure that the process cache entry of the process was properly deleted from the cache
	err = retry.Do(func() error {
		if !ebpfLessEnabled {
			p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
			if !ok {
				t.Skip("not supported")
			}
			entry := p.Resolvers.ProcessResolver.Get(execPid)
			if entry != nil {
				return errors.New("the process cache entry was not deleted from the user space cache")
			}
		} else {
			p, ok := test.probe.PlatformProbe.(*sprobe.EBPFLessProbe)
			if !ok {
				t.Skip("not supported")
			}
			entry := p.Resolvers.ProcessResolver.Resolve(process.CacheResolverKey{Pid: execPid, NSID: nsID})
			if entry != nil {
				return errors.New("the process cache entry was not deleted from the user space cache")
			}
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestProcessCredentialsUpdate(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_setuid",
			Expression: `setuid.uid == 1001 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setreuid",
			Expression: `setuid.uid == 1002 && setuid.euid == 1003 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setfsuid",
			Expression: `setuid.fsuid == 1004 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setgid",
			Expression: `setgid.gid == 1005 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setregid",
			Expression: `setgid.gid == 1006 && setgid.egid == 1007 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setfsgid",
			Expression: `setgid.fsgid == 1008 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_capset",
			Expression: `capset.cap_effective & CAP_WAKE_ALARM == 0 && capset.cap_permitted & CAP_SYS_BOOT == 0 && process.file.name == "syscall_go_tester"`,
		},
		{
			ID:         "test_auid",
			Expression: `exec.auid == 1234`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("setuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setuid", "1001", "0")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setuid")
			assert.Equal(t, uint32(1001), event.SetUID.UID, "wrong uid")
		})
	})

	t.Run("setreuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setreuid", "1002", "1003")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setreuid")
			assert.Equal(t, uint32(1002), event.SetUID.UID, "wrong uid")
			assert.Equal(t, uint32(1003), event.SetUID.EUID, "wrong euid")
		})
	})

	t.Run("setresuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setresuid", "1002", "1003")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setreuid")
			assert.Equal(t, uint32(1002), event.SetUID.UID, "wrong uid")
			assert.Equal(t, uint32(1003), event.SetUID.EUID, "wrong euid")
		})
	})

	t.Run("setfsuid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setfsuid", "1004", "0")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setfsuid")
			assert.Equal(t, uint32(1004), event.SetUID.FSUID, "wrong fsuid")
		})
	})

	t.Run("setgid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setgid", "1005", "0")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setgid")
			assert.Equal(t, uint32(1005), event.SetGID.GID, "wrong gid")
		})
	})

	t.Run("setregid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setregid", "1006", "1007")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setregid")
			assert.Equal(t, uint32(1006), event.SetGID.GID, "wrong gid")
			assert.Equal(t, uint32(1007), event.SetGID.EGID, "wrong egid")
		})
	})

	t.Run("setresgid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setresgid", "1006", "1007")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setregid")
			assert.Equal(t, uint32(1006), event.SetGID.GID, "wrong gid")
			assert.Equal(t, uint32(1007), event.SetGID.EGID, "wrong egid")
		})
	})

	t.Run("setfsgid", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "process-credentials", "setfsgid", "1008", "0")
		}, func(event *model.Event, rule *rules.Rule) {
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
		// remove capabilities that are removed by syscall_go_tester
		threadCapabilities.Unset(capability.PERMITTED|capability.EFFECTIVE, capability.CAP_SYS_BOOT)
		threadCapabilities.Unset(capability.EFFECTIVE, capability.CAP_WAKE_ALARM)

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, goSyscallTester, "-process-credentials-capset")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_capset")

			// transform the collected kernel capabilities into a usable capability set
			newSet, err := capability.NewPid2(0)
			if err != nil {
				t.Error(err)
			}
			newSet.Clear(capability.PERMITTED | capability.EFFECTIVE)
			parseCapIntoSet(event.Capset.CapEffective, capability.EFFECTIVE, newSet)
			parseCapIntoSet(event.Capset.CapPermitted, capability.PERMITTED, newSet)

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
}

func parseCapIntoSet(capabilities uint64, flag capability.CapType, c capability.Capabilities) {
	for _, v := range model.KernelCapabilityConstants {
		if v == 0 {
			continue
		}

		if capabilities&v == v {
			c.Set(flag, capability.Cap(bits.TrailingZeros64(v)))
		}
	}
}

func TestProcessIsThread(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_process_fork_is_thread",
			Expression: `open.file.path == "/dev/null" && process.file.name == "syscall_tester" && process.ancestors.file.name == "syscall_tester" && process.is_thread`,
		},
		{
			ID:         "test_process_exec_is_not_thread",
			Expression: `open.file.path == "/dev/null" && process.file.name in ["syscall_tester", "exe"] && process.ancestors.file.name == "syscall_tester" && !process.is_thread`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fork-is-not-exec", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"fork"}
			cmd := exec.Command(syscallTester, args...)
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_process_fork_is_thread")
			assert.Equal(t, "syscall_tester", event.ProcessContext.FileEvent.BasenameStr, "wrong process file basename")
			assert.Equal(t, "syscall_tester", event.ProcessContext.Ancestor.ProcessContext.FileEvent.BasenameStr, "wrong parent process file basename")
			assert.Equal(t, "syscall_tester", event.ProcessContext.Parent.FileEvent.BasenameStr, "wrong parent process file basename")
			assert.False(t, event.ProcessContext.IsExec, "process shouldn't be marked as being an exec")
		})
	})

	t.Run("exec-is-exec", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"fork", "exec"}
			cmd := exec.Command(syscallTester, args...)
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_process_exec_is_not_thread")
			assert.Equal(t, "syscall_tester", event.ProcessContext.Ancestor.ProcessContext.FileEvent.BasenameStr, "wrong parent process file basename")
			assert.Equal(t, "syscall_tester", event.ProcessContext.Parent.FileEvent.BasenameStr, "wrong parent process file basename")
			assert.True(t, event.ProcessContext.IsExec, "process should be marked as being an exec")
		})
	})
}

func TestProcessExit(t *testing.T) {
	SkipIfNotAvailable(t)

	sleepExec := which(t, "sleep")
	timeoutExec := which(t, "timeout")

	const envpExitSleep = "TESTPROCESSEXITSLEEP=1"
	const envpExitSleepTime = "TESTPROCESSEXITSLEEPTIME=1"

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_exit_ok",
			Expression: fmt.Sprintf(`exit.cause == EXITED && exit.code == 0 && process.file.path == "%s" && process.envp in ["%s"]`, sleepExec, envpExitSleep),
		},
		{
			ID:         "test_exit_error",
			Expression: fmt.Sprintf(`exit.cause == EXITED && exit.code == 1 && process.file.path == "%s" && process.envp in ["%s"]`, sleepExec, envpExitSleep),
		},
		{
			ID:         "test_exit_coredump",
			Expression: fmt.Sprintf(`exit.cause == COREDUMPED && exit.code == SIGQUIT && process.file.path == "%s" && process.envp in ["%s"]`, sleepExec, envpExitSleep),
		},
		{
			ID:         "test_exit_signal",
			Expression: fmt.Sprintf(`exit.cause == SIGNALED && exit.code == SIGKILL && process.file.path == "%s" && process.envp in ["%s"]`, sleepExec, envpExitSleep),
		},
		{
			ID:         "test_exit_time_1",
			Expression: fmt.Sprintf(`exit.cause == EXITED && exit.code == 0 && process.created_at < 4s && process.file.path == "%s" && process.envp in ["%s"]`, sleepExec, envpExitSleepTime),
		},
		{
			ID:         "test_exit_time_2",
			Expression: fmt.Sprintf(`exit.cause == EXITED && exit.code == 0 && process.created_at > 4s && process.file.path == "%s" && process.envp in ["%s"]`, sleepExec, envpExitSleepTime),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("exit-ok", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"0"}
			envp := []string{envpExitSleep}

			cmd := exec.Command(sleepExec, args...)
			cmd.Env = envp
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			test.validateExitSchema(t, event)
			assertTriggeredRule(t, rule, "test_exit_ok")
			assertFieldEqual(t, event, "exit.file.path", sleepExec)
			assert.Equal(t, uint32(model.ExitExited), event.Exit.Cause, "wrong exit cause")
			assert.Equal(t, uint32(0), event.Exit.Code, "wrong exit code")
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
		})
	})

	t.Run("exit-error", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{} // sleep with no argument should exit with return code 1
			envp := []string{envpExitSleep}

			cmd := exec.Command(sleepExec, args...)
			cmd.Env = envp
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			test.validateExitSchema(t, event)
			assertTriggeredRule(t, rule, "test_exit_error")
			assertFieldEqual(t, event, "exit.file.path", sleepExec)
			assert.Equal(t, uint32(model.ExitExited), event.Exit.Cause, "wrong exit cause")
			assert.Equal(t, uint32(1), event.Exit.Code, "wrong exit code")
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
		})
	})

	t.Run("exit-coredumped", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"--preserve-status", "--signal=SIGQUIT", "2", sleepExec, "9"}
			envp := []string{envpExitSleep}

			cmd := exec.Command(timeoutExec, args...)
			cmd.Env = envp
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			test.validateExitSchema(t, event)
			assertTriggeredRule(t, rule, "test_exit_coredump")
			assertFieldEqual(t, event, "exit.file.path", sleepExec)
			assert.Equal(t, uint32(model.ExitCoreDumped), event.Exit.Cause, "wrong exit cause")
			assert.Equal(t, uint32(syscall.SIGQUIT), event.Exit.Code, "wrong exit code")
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
		})
	})

	t.Run("exit-signaled", func(t *testing.T) {
		SkipIfNotAvailable(t)

		test.WaitSignal(t, func() error {
			args := []string{"--preserve-status", "--signal=SIGKILL", "2", sleepExec, "9"}
			envp := []string{envpExitSleep}

			cmd := exec.Command(timeoutExec, args...)
			cmd.Env = envp
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			test.validateExitSchema(t, event)
			assertTriggeredRule(t, rule, "test_exit_signal")
			assertFieldEqual(t, event, "exit.file.path", sleepExec)
			assert.Equal(t, uint32(model.ExitSignaled), event.Exit.Cause, "wrong exit cause")
			assert.Equal(t, uint32(syscall.SIGKILL), event.Exit.Code, "wrong exit code")
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
		})
	})

	t.Run("exit-time-1", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"--preserve-status", "--signal=SIGKILL", "9", sleepExec, "2"}
			envp := []string{envpExitSleepTime}

			cmd := exec.Command(timeoutExec, args...)
			cmd.Env = envp
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			test.validateExitSchema(t, event)
			assertTriggeredRule(t, rule, "test_exit_time_1")
			assertFieldEqual(t, event, "exit.file.path", sleepExec)
			assert.Equal(t, uint32(model.ExitExited), event.Exit.Cause, "wrong exit cause")
			assert.Equal(t, uint32(0), event.Exit.Code, "wrong exit code")
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
		})
	})

	t.Run("exit-time-2", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			args := []string{"--preserve-status", "--signal=SIGKILL", "9", sleepExec, "5"}
			envp := []string{envpExitSleepTime}

			cmd := exec.Command(timeoutExec, args...)
			cmd.Env = envp
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			test.validateExitSchema(t, event)
			assertTriggeredRule(t, rule, "test_exit_time_2")
			assertFieldEqual(t, event, "exit.file.path", sleepExec)
			assert.Equal(t, uint32(model.ExitExited), event.Exit.Cause, "wrong exit cause")
			assert.Equal(t, uint32(0), event.Exit.Code, "wrong exit code")
			assert.False(t, event.ProcessContext.ExitTime.Before(event.ProcessContext.ExecTime), "exit time < exec time")
		})
	})
}

func TestProcessBusybox(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_busybox_1",
			Expression: `exec.file.path == "/usr/bin/whoami"`,
		},
		{
			ID:         "test_busybox_2",
			Expression: `exec.file.path == "/bin/sync"`,
		},
		{
			ID:         "test_busybox_3",
			Expression: `exec.file.name == "df"`,
		},
		{
			ID:         "test_busybox_4",
			Expression: `open.file.path == "/tmp/busybox-test" && process.file.name == "touch"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	wrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "alpine", "")
	if err != nil {
		t.Skip("docker no available")
		return
	}

	wrapper.Run(t, "busybox-1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/usr/bin/whoami", nil, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_busybox_1", rule.ID, "wrong rule triggered")
		})
	})

	wrapper.Run(t, "busybox-2", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/bin/sync", nil, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_busybox_2", rule.ID, "wrong rule triggered")
		})
	})

	wrapper.Run(t, "busybox-3", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/bin/df", nil, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_busybox_3", rule.ID, "wrong rule triggered")
		})
	})

	wrapper.Run(t, "busybox-4", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/bin/touch", []string{"/tmp/busybox-test"}, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "test_busybox_4", rule.ID, "wrong rule triggered")
		})
	})
}

func TestProcessInterpreter(t *testing.T) {
	SkipIfNotAvailable(t)

	python, whichPythonErr := whichNonFatal("python")
	if whichPythonErr != nil {
		python = which(t, "python3")
	}

	tests := []struct {
		name            string
		rule            *rules.RuleDefinition
		scriptName      string
		executedScript  string
		innerScriptName string
		check           func(event *model.Event)
	}{
		{
			name: "regular exec",
			rule: &rules.RuleDefinition{
				ID:         "test_regular_exec",
				Expression: `exec.file.name == ~"*python*" && process.ancestors.interpreter.file.name == "bash" && process.ancestors.file.name == "testsuite"`, // RHEL python is platform-python3.6
			},
			scriptName: "regularExec.sh",
			executedScript: fmt.Sprintf(`#!/bin/bash

echo "Executing echo insIDe a bash script"

%s - << EOF
print('Executing print insIDe a python (%s) script insIDe a bash script')

EOF

echo "Back to bash"`, python, python),
			check: func(event *model.Event) {
				var fieldNotSupportedError *eval.ErrNotSupported
				_, err := event.GetFieldValue("exec.interpreter.file.name")
				assert.ErrorAs(t, err, &fieldNotSupportedError, "exec event shouldn't have an interpreter")
				assertFieldEqual(t, event, "process.parent.file.name", "regularExec.sh", "wrong process parent file name")
				assertFieldStringArrayIndexedOneOf(t, event, "process.ancestors.file.name", 0, []string{"regularExec.sh"}, "ancestor file name not an option")
			},
		},
		{
			name: "regular exec without interpreter rule",
			rule: &rules.RuleDefinition{
				ID:         "test_regular_exec_without_interpreter_rule",
				Expression: `exec.file.name == ~"*python*" && exec.interpreter.file.name == ""`,
			},
			scriptName: "regularExecWithInterpreterRule.sh",
			executedScript: fmt.Sprintf(`#!/bin/bash

echo "Executing echo insIDe a bash script"

%s <<__HERE__
#!%s

print('Executing print insIDe a python (%s) script insIDe a bash script')
__HERE__

echo "Back to bash"`, python, python, python),
			check: func(event *model.Event) {
				var fieldNotSupportedError *eval.ErrNotSupported
				_, err := event.GetFieldValue("exec.interpreter.file.name")
				assert.ErrorAs(t, err, &fieldNotSupportedError, "exec event shouldn't have an interpreter")
				assertFieldEqual(t, event, "process.parent.file.name", "regularExecWithInterpreterRule.sh", "wrong process parent file name")
				assertFieldStringArrayIndexedOneOf(t, event, "process.ancestors.file.name", 0, []string{"regularExecWithInterpreterRule.sh"}, "ancestor file name not an option")
			},
		},
		{
			name: "interpreted exec",
			rule: &rules.RuleDefinition{
				ID:         "test_interpreted_event",
				Expression: `exec.interpreter.file.name == ~"*python*" && exec.file.name == "pyscript.py"`,
			},
			scriptName:      "interpretedExec.sh",
			innerScriptName: "pyscript.py",
			executedScript: fmt.Sprintf(`#!/bin/bash

echo "Executing echo insIDe a bash script"

cat << EOF > pyscript.py
#!%s

print('Executing print insIDe a python (%s) script inside a bash script')

EOF

echo "Back to bash"

chmod 755 pyscript.py
./pyscript.py`, python, python),
			check: func(event *model.Event) {
				assertFieldEqual(t, event, "exec.interpreter.file.name", filepath.Base(python), "wrong interpreter file name")
				assertFieldEqual(t, event, "process.parent.file.name", "interpretedExec.sh", "wrong process parent file name")
				assertFieldStringArrayIndexedOneOf(t, event, "process.ancestors.file.name", 0, []string{"interpretedExec.sh"}, "ancestor file name not an option")
			},
		},
		// TODO: Test for snapshotted processes
		// TODO: Nested interpreted exec is unsupported for now
		//		{
		//			name: "nested interpreted exec",
		//			rule: &rules.RuleDefinition{
		//				ID:         "test_nested_interpreted_event",
		//				Expression: fmt.Sprintf(`exec.interpreter.file.name == ~"perl"`),
		//			},
		//			scriptName: "nestedInterpretedExec.sh",
		//			executedScript: `#!/bin/bash
		//
		//echo "Executing echo insIDe a bash script"
		//
		//cat << '__HERE__' > hello.pl
		//#!/usr/bin/perl
		//
		//my $foo = "Hello from Perl";
		//print "$foo\n";
		//
		//__HERE__
		//
		//chmod 755 hello.pl
		//
		//cat << EOF > pyscript.py
		//#!/usr/bin/python3
		//
		//import subprocess
		//
		//print('Executing print insIDe a python script')
		//
		//subprocess.run(["perl", "./hello.pl"])
		//
		//EOF
		//
		//echo "Back to bash"
		//
		//chmod 755 pyscript.py
		//./pyscript.py`,
		//		},
	}

	var ruleList []*rules.RuleDefinition
	for _, test := range tests {
		test.rule.Expression += "  && process.parent.file.name == \"" + test.scriptName + "\""
		ruleList = append(ruleList, test.rule)
	}

	testModule, err := newTestModule(t, nil, ruleList)
	if err != nil {
		t.Fatal(err)
	}
	defer testModule.Close()

	p, ok := testModule.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	for _, test := range tests {
		testModule.Run(t, test.name, func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			scriptLocation := filepath.Join(os.TempDir(), test.scriptName)
			if scriptWriteErr := os.WriteFile(scriptLocation, []byte(test.executedScript), 0755); scriptWriteErr != nil {
				t.Fatalf("could not write %s: %s", scriptLocation, scriptWriteErr)
			}
			defer os.Remove(scriptLocation)
			defer os.Remove(test.innerScriptName) // script created by script is in working directory

			testModule.WaitSignal(t, func() error {
				cmd := exec.Command(scriptLocation)
				cmd.Dir = os.TempDir()
				output, scriptRunErr := cmd.CombinedOutput()
				if scriptRunErr != nil {
					t.Errorf("could not run %s: %s", scriptLocation, scriptRunErr)
				}
				t.Logf(string(output))

				offsets, _ := p.GetOffsetConstants()
				t.Logf("%s: %+v\n", constantfetch.OffsetNameLinuxBinprmStructFile, offsets[constantfetch.OffsetNameLinuxBinprmStructFile])

				return nil
			}, testModule.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, test.rule.ID)
				test.check(event)
			}))
		})
	}
}

func TestProcessResolution(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_resolution",
			Expression: `open.file.path == "/tmp/test-process-resolution"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	var cmd *exec.Cmd
	var stdin io.WriteCloser
	defer func() {
		if stdin != nil {
			stdin.Close()
		}

		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	test.WaitSignal(t, func() error {
		var err error

		args := []string{"open", "/tmp/test-process-resolution", ";",
			"getchar", ";",
			"open", "/tmp/test-process-resolution"}

		cmd = exec.Command(syscallTester, args...)
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return err
		}

		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_resolution", rule.ID, "wrong rule triggered")

		value, err := event.GetFieldValue("process.pid")
		if err != nil {
			t.Errorf("not able to get pid")
		}
		pid := uint32(value.(int))

		value, err = event.GetFieldValue("process.file.inode")
		if err != nil {
			t.Errorf("not able to get pid")
		}
		inode := uint64(value.(int))

		resolver := p.Resolvers.ProcessResolver

		// compare only few fields as the hierarchy fields(pointers, etc) are modified by the resolution function calls
		equals := func(t *testing.T, entry1, entry2 *model.ProcessCacheEntry) {
			t.Helper()

			assert.NotNil(t, entry1)
			assert.NotNil(t, entry2)
			assert.Equal(t, entry1.FileEvent.PathnameStr, entry2.FileEvent.PathnameStr)
			assert.Equal(t, entry1.Pid, entry2.Pid)
			assert.Equal(t, entry1.PPid, entry2.PPid)
			assert.Equal(t, entry1.ContainerID, entry2.ContainerID)
			assert.Equal(t, entry1.Cookie, entry2.Cookie)

			// may not be exactly equal because of clock drift between two boot time resolution, see time resolver
			assert.Greater(t, time.Second, entry1.ExecTime.Sub(entry2.ExecTime).Abs())
			assert.Greater(t, time.Second, entry1.ForkTime.Sub(entry2.ForkTime).Abs())
		}

		cacheEntry := resolver.ResolveFromCache(pid, pid, inode)
		if cacheEntry == nil {
			t.Errorf("not able to resolve the entry")
		}

		mapsEntry := resolver.ResolveFromKernelMaps(pid, pid, inode)
		if mapsEntry == nil {
			t.Errorf("not able to resolve the entry")
		}

		equals(t, cacheEntry, mapsEntry)

		// This makes use of the cache and do not parse /proc
		// it still checks the ResolveFromProcfs returns the correct entry
		procEntry := resolver.ResolveFromProcfs(pid)
		if procEntry == nil {
			t.Fatalf("not able to resolve the entry")
		}

		equals(t, mapsEntry, procEntry)

		io.WriteString(stdin, "\n")
	})
}

func TestProcessFilelessExecution(t *testing.T) {
	SkipIfNotAvailable(t)

	filelessDetectionRule := &rules.RuleDefinition{
		ID:         "test_fileless",
		Expression: fmt.Sprintf(`exec.file.name == "%s" && exec.file.path == ""`, filelessExecutionFilenamePrefix),
	}

	filelessWithInterpreterDetectionRule := &rules.RuleDefinition{
		ID:         "test_fileless_with_interpreter",
		Expression: fmt.Sprintf(`exec.file.name == "%sscript" && exec.file.path == "" && exec.interpreter.file.name == "bash"`, filelessExecutionFilenamePrefix),
	}

	tests := []struct {
		name                             string
		rule                             *rules.RuleDefinition
		syscallTesterToRun               string
		syscallTesterScriptFilenameToRun string
		check                            func(event *model.Event, rule *rules.Rule)
	}{
		{
			name:                             "fileless",
			rule:                             filelessDetectionRule,
			syscallTesterToRun:               "fileless",
			syscallTesterScriptFilenameToRun: "",
			check: func(event *model.Event, rule *rules.Rule) {
				assertFieldEqual(
					t, event, "process.file.name", filelessExecutionFilenamePrefix, "process.file.name not matching",
				)
				assertFieldStringArrayIndexedOneOf(
					t, event, "process.ancestors.file.name", 0, []string{"syscall_tester"},
					"process.ancestors.file.name not matching",
				)
			},
		},
		{
			name:                             "fileless with script name",
			rule:                             filelessWithInterpreterDetectionRule,
			syscallTesterToRun:               "fileless",
			syscallTesterScriptFilenameToRun: "script",
			check: func(event *model.Event, rule *rules.Rule) {
				assertFieldEqual(t, event, "process.file.name", "memfd:script", "process.file.name not matching")
			},
		},
		{
			name:               "real file with fileless prefix",
			syscallTesterToRun: "none",
			rule:               filelessDetectionRule,
		},
	}

	var ruleList []*rules.RuleDefinition
	alreadyAddedRules := make(map[string]bool)
	for _, test := range tests {
		if _, ok := alreadyAddedRules[test.rule.ID]; !ok {
			alreadyAddedRules[test.rule.ID] = true
			ruleList = append(ruleList, test.rule)
		}
	}

	testModule, err := newTestModule(t, nil, ruleList)
	if err != nil {
		t.Fatal(err)
	}
	defer testModule.Close()

	syscallTester, err := loadSyscallTester(t, testModule, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.syscallTesterToRun == "none" {
				err = testModule.GetSignal(t, func() error {
					fileMode := 0o477
					testFile, _, err := testModule.CreateWithOptions(filelessExecutionFilenamePrefix, 98, 99, fileMode)
					if err != nil {
						return err
					}
					defer os.Remove(testFile)

					f, err := os.OpenFile(testFile, os.O_WRONLY, 0)
					if err != nil {
						t.Fatal(err)
					}
					f.WriteString("#!/bin/bash")
					f.Close()

					cmd := exec.Command(testFile)
					return cmd.Run()
				}, func(event *model.Event, rule *rules.Rule) {
					t.Errorf("shouldn't get an event: got event: %s", testModule.debugEvent(event))
				})
				if err == nil {
					t.Fatal("shouldn't get an event")
				}
			} else {
				if ebpfLessEnabled && test.rule.ID == "test_fileless_with_interpreter" {
					t.Skip("interpreter detection unsupported")
				}

				testModule.WaitSignal(t, func() error {
					return runSyscallTesterFunc(context.Background(), t, syscallTester, test.syscallTesterToRun, test.syscallTesterScriptFilenameToRun)
				}, func(event *model.Event, rule *rules.Rule) {
					assertTriggeredRule(t, rule, test.rule.ID)
					test.check(event, rule)
				})
			}
		})
	}
}
