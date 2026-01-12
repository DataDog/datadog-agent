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

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestFIMOpen(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_fim_rule",
			Expression: `fim.write.file.path == "{{.Root}}/test-open"`,
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
	defer os.Remove(testFile)

	// open test
	test.WaitSignalFromRule(t, func() error {
		f, err := os.Create(testFile)
		if err != nil {
			return err
		}
		return f.Close()
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
		assertTriggeredRule(t, rule, "__fim_expanded_open__test_fim_rule")
		assert.Equal(t, rule.Def.ID, "test_fim_rule")
		assertInode(t, event.Open.File.Inode, getInode(t, testFile))
	}, "__fim_expanded_open__test_fim_rule")

	// chmod test
	test.WaitSignalFromRule(t, func() error {
		return os.Chmod(testFile, 0o777)
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "chmod", event.GetType(), "wrong event type")
		assertTriggeredRule(t, rule, "__fim_expanded_chmod__test_fim_rule")
		assert.Equal(t, rule.Def.ID, "test_fim_rule")
		assertInode(t, event.Chmod.File.Inode, getInode(t, testFile))
	}, "__fim_expanded_chmod__test_fim_rule")

	// open but read only
	_ = test.GetSignal(t, func() error {
		f, err := os.Open(testFile)
		if err != nil {
			return err
		}
		return f.Close()
	}, func(_ *model.Event, _ *rules.Rule) {
		t.Error("Event received (rule is in write only mode, and the open is read only)")
	})
}

func TestFIMPermError(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_perm_open_rule",
			Expression: `open.file.path == "{{.Root}}/test-file" && open.retval == -13`,
		},
		{
			ID:         "test_perm_unlink_rule",
			Expression: `unlink.file.path == "{{.Root}}/test-file" && unlink.retval == -13`,
		},
		{
			ID:         "test_perm_chmod_rule",
			Expression: `chmod.file.path == "{{.Root}}/test-file" && chmod.retval == -1`,
		},
		{
			ID:         "test_perm_chown_rule",
			Expression: `chown.file.path == "{{.Root}}/test-file" && chown.retval == -1`,
		},
		{
			ID:         "test_perm_rename_rule",
			Expression: `rename.file.destination.path == "{{.Root}}/test-file" && rename.file.path == "{{.Root}}/rename-file" && rename.retval == -13`,
		},
		{
			ID:         "test_perm_utimes_rule",
			Expression: `utimes.file.path == "{{.Root}}/test-file" && utimes.retval == -1`,
		},
		{
			ID:         "test_perm_link_rule",
			Expression: `link.file.path == "{{.Root}}/test-file" && link.file.destination.path == "{{.Root}}/link-file" && link.retval == -1`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-file")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chmod(testFile, 0o400)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chown(testFile, 2002, 2002)
	if err != nil {
		t.Fatal(err)
	}

	renameFile, _, err := test.Create("rename-file")
	if err != nil {
		t.Fatal(err)
	}

	linkFile, _, err := test.Path("link-file")
	if err != nil {
		t.Fatal(err)
	}

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// Set the fs.protected_hardlinks sysctl to 1 to cause the link syscall to fail with EPERM
	// when the user is not the owner of the source file.
	const fsProcHardlinks = "fs/protected_hardlinks"
	initialValue, err := readSysctlValue(fsProcHardlinks)
	if err != nil {
		t.Fatalf("couldn't read %s: %v", fsProcHardlinks, err)
	}
	defer func() {
		// reset initial value
		if err = writeSysctlValue(fsProcHardlinks, initialValue); err != nil {
			t.Fatalf("couldn't reset %s to %s: %v", fsProcHardlinks, initialValue, err)
		}
	}()

	// make sure the correct value is set before the test starts
	if err := writeSysctlValue(fsProcHardlinks, "1"); err != nil {
		t.Fatalf("couldn't set %s: %v", fsProcHardlinks, err)
	}

	test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"open", testFile,
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_open_rule")
			assert.Equal(t, -int64(syscall.EACCES), event.Open.Retval)
		}, "test_perm_open_rule")
	})

	test.RunMultiMode(t, "unlink", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"unlink", testFile,
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_unlink_rule")
			assert.Equal(t, -int64(syscall.EACCES), event.Unlink.Retval)
		}, "test_perm_unlink_rule")
	})

	test.RunMultiMode(t, "chmod", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"chmod", testFile, "0600",
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_chmod_rule")
			assert.Equal(t, -int64(syscall.EPERM), event.Chmod.Retval)
		}, "test_perm_chmod_rule")
	})

	test.RunMultiMode(t, "chown", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"chown", testFile, "0", "0",
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_chown_rule")
			assert.Equal(t, -int64(syscall.EPERM), event.Chown.Retval)
		}, "test_perm_chown_rule")
	})

	test.RunMultiMode(t, "rename", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"rename", renameFile, testFile,
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_rename_rule")
			assert.Equal(t, -int64(syscall.EACCES), event.Rename.Retval)
		}, "test_perm_rename_rule")
	})

	test.RunMultiMode(t, "utimes", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"utimes", testFile,
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_utimes_rule")
			assert.Equal(t, -int64(syscall.EPERM), event.Utimes.Retval)
		}, "test_perm_utimes_rule")
	})

	test.RunMultiMode(t, "link", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{
			"process-credentials", "setuid", "4001", "4001", ";",
			"link", testFile, linkFile,
		}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_perm_link_rule")
			assert.Equal(t, -int64(syscall.EPERM), event.Link.Retval)
		}, "test_perm_link_rule")
	})
}
