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
	"os/user"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/avast/retry-go"
	"github.com/pkg/errors"
	"kernel.org/pub/linux/libs/security/libcap/cap"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestProcess(t *testing.T) {
	currentUser, err := user.LookupId("0")
	if err != nil {
		t.Fatal(err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`process.file.user == "%s" && process.file.name == "%s" && open.file.path == "{{.Root}}/test-process"`, currentUser.Name, path.Base(executable)),
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
		if rule.ID != "test_rule" {
			t.Errorf("expected rule 'test-rule' to be triggered, got %s", rule.ID)
		}
	}
}

func TestProcessContext(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `open.file.path == "{{.Root}}/test-process-context" && open.flags & O_CREAT == 0`,
		},
		{
			ID:         "test_rule_ancestors",
			Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-process-ancestors" && process.ancestors[_].file.name == "%s"`, path.Base(executable)),
		},
	}

	test, err := newTestModule(nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

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

	t.Run("inode", func(t *testing.T) {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if filename, _ := event.GetFieldValue("process.file.path"); filename.(string) != executable {
				t.Errorf("not able to find the proper process filename `%v` vs `%s`: %v", filename, executable, event)
			}

			if inode := getInode(t, executable); inode != event.Process.FileFields.Inode {
				t.Logf("expected inode %d, got %d", event.Process.FileFields.Inode, inode)
			}

			testContainerPath(t, event, "process.file.container_path")
		}
	})

	t.Run("tty", func(t *testing.T) {
		// not working on centos8
		t.Skip()

		executable := "/usr/bin/cat"
		if resolved, err := os.Readlink(executable); err == nil {
			executable = resolved
		} else {
			if os.IsNotExist(err) {
				executable = "/bin/cat"
			}
		}

		cmd := exec.Command("script", "/dev/null", "-c", executable+" "+testFile)
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Error(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if filename, _ := event.GetFieldValue("process.file.path"); filename.(string) != executable {
				t.Errorf("not able to find the proper process filename `%v` vs `%s`", filename, executable)
			}

			if name, _ := event.GetFieldValue("process.tty_name"); name.(string) == "" {
				t.Error("not able to get a tty name")
			}

			if inode := getInode(t, executable); inode != event.Process.FileFields.Inode {
				t.Logf("expected inode %d, got %d", event.Process.FileFields.Inode, inode)
			}

			testContainerPath(t, event, "process.file.container_path")
		}
	})

	t.Run("ancestors", func(t *testing.T) {
		shell := "/usr/bin/sh"
		if resolved, err := os.Readlink(shell); err == nil {
			shell = resolved
		}
		shell = path.Base(shell)

		executable := "/usr/bin/touch"
		if resolved, err := os.Readlink(executable); err == nil {
			executable = resolved
		} else {
			if os.IsNotExist(err) {
				executable = "/bin/touch"
			}
		}

		testFile, _, err := test.Path("test-process-ancestors")
		if err != nil {
			t.Fatal(err)
		}

		// Bash attempts to optimize away forks in the last command in a function body
		// under appropriate circumstances (source: bash changelog)
		cmd := exec.Command(shell, "-c", "$("+executable+" "+testFile+")")
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Error(err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if filename := event.ResolveExecInode(&event.Exec); filename != executable {
				t.Errorf("expected process filename `%s`, got `%s`: %v", executable, filename, event)
			}

			if rule.ID != "test_rule_ancestors" {
				t.Error("Wrong rule triggered")
			}

			if ancestor := event.Process.Ancestor; ancestor == nil || ancestor.Comm != shell {
				t.Errorf("ancestor `%s` expected, got %v, event:%v", shell, ancestor, event)
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
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Error(err)
	}

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if filename, _ := event.GetFieldValue("exec.file.path"); filename.(string) != executable {
			t.Errorf("expected exec filename `%v`, got `%v`", executable, filename)
		}

		if filename, _ := event.GetFieldValue("process.file.path"); filename.(string) != executable {
			t.Errorf("expected process filename `%v`, got `%v`", executable, filename)
		}

		testContainerPath(t, event, "exec.file.container_path")
	}
}

func TestProcessMetadata(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `exec.file.path == "{{.Root}}/test-exec" && exec.file.uid == 98 && exec.file.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
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
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Error(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "exec" {
				t.Errorf("expected exec event, got %s", event.GetType())
			}

			if int(event.Exec.FileFields.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Exec.FileFields.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Exec.FileFields.MTime.After(now) || event.Exec.FileFields.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Exec.FileFields.MTime)
			}

			if event.Exec.FileFields.CTime.After(now) || event.Exec.FileFields.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Exec.FileFields.CTime)
			}
		}
	})

	t.Run("credentials", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 2001, 2002, 0); errno != 0 {
				t.Fatal(errno)
			}
			if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 1001, 1002, 0); errno != 0 {
				t.Fatal(errno)
			}

			cmd := exec.Command(testFile)
			if _, err := cmd.CombinedOutput(); err != nil {
				t.Error(err)
			}
		}()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "exec" {
				t.Errorf("expected exec event, got %s", event.GetType())
			}

			if uid := event.ResolveCredentialsUID(&event.Exec.Credentials); uid != 1001 {
				t.Errorf("expected uid 1001, got %d", uid)
			}
			if euid := event.ResolveCredentialsEUID(&event.Exec.Credentials); euid != 1002 {
				t.Errorf("expected euid 1002, got %d", euid)
			}
			if fsuid := event.ResolveCredentialsFSUID(&event.Process.Credentials); fsuid != 1002 {
				t.Errorf("expected fsuid 1002, got %d", fsuid)
			}

			if gid := event.ResolveCredentialsGID(&event.Exec.Credentials); gid != 2001 {
				t.Errorf("expected gid 2001, got %d", gid)
			}
			if egid := event.ResolveCredentialsEGID(&event.Exec.Credentials); egid != 2002 {
				t.Errorf("expected egid 2002, got %d", egid)
			}
			if fsgid := event.ResolveCredentialsFSGID(&event.Exec.Credentials); fsgid != 2002 {
				t.Errorf("expected fsgid 2002, got %d", fsgid)
			}
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
		Expression: fmt.Sprintf(`exec.file.path == "%s"`, executable),
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{wantProbeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	cmd := exec.Command(executable, "/dev/null")
	if _, err := cmd.CombinedOutput(); err != nil {
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
				execPid = int(event.Process.Pid)
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
				if int(event.Process.Pid) == execPid {
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
			if cacheEntry.ID != cacheEntry.Ancestor.ID {
				t.Errorf("expected container ID %s, got %s", cacheEntry.Ancestor.ID, cacheEntry.ID)
			}
		}
	}

	testContainerPath(t, event, "process.file.container_path")
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
			if newEntry.Cookie != parentEntry.Cookie {
				t.Errorf("expected cookie %d, %d", parentEntry.Cookie, newEntry.Cookie)
			}

			if newEntry.PPid != parentEntry.Pid {
				t.Errorf("expected PPid %d, got %d", parentEntry.Pid, newEntry.PPid)
			}

			// we also need to check the container ID lineage
			if newEntry.ID != parentEntry.ID {
				t.Errorf("expected container ID %s, got %s", parentEntry.ID, newEntry.ID)
			}

			// We can't check that the new entry is in the list of the children of its parent because the exit event
			// has probably already been processed (thus the parent list of children has already been updated and the
			// child entry deleted).
		}

		testContainerPath(t, event, "process.file.container_path")
	}
}

func testProcessLineageExit(t *testing.T, event *probe.Event, test *testModule) {
	// make sure that the process cache entry of the process was properly deleted from the cache
	err := retry.Do(func() error {
		resolvers := test.probe.GetResolvers()
		entry := resolvers.ProcessResolver.Get(event.Process.Pid)
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
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setuid" {
				t.Errorf("expected test_setuid rule, got %s", rule.ID)
			}

			if event.SetUID.UID != 1001 {
				t.Errorf("expected uid 1001, got %d", event.SetUID.UID)
			}
		}
	})

	t.Run("setreuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 1002, 1003, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setreuid" {
				t.Errorf("expected test_setreuid rule, got %s", rule.ID)
			}

			if event.SetUID.UID != 1002 {
				t.Errorf("expected uid 1002, got %d", event.SetUID.UID)
			}
			if event.SetUID.EUID != 1003 {
				t.Errorf("expected euid 1003, got %d", event.SetUID.EUID)
			}
		}
	})

	t.Run("setresuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETRESUID, 1002, 1003, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setreuid" {
				t.Errorf("expected test_setreuid rule, got %s", rule.ID)
			}

			if event.SetUID.UID != 1002 {
				t.Errorf("expected uid 1002, got %d", event.SetUID.UID)
			}
			if event.SetUID.EUID != 1003 {
				t.Errorf("expected euid 1003, got %d", event.SetUID.EUID)
			}
		}
	})

	t.Run("setfsuid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETFSUID, 1004, 0, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setfsuid" {
				t.Errorf("expected test_setfsuid rule, got %s", rule.ID)
			}

			if event.SetUID.FSUID != 1004 {
				t.Errorf("expected fsuid 1004, got %d", event.SetUID.FSUID)
			}
		}
	})

	t.Run("setgid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETGID, 1005, 0, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setgid" {
				t.Errorf("expected test_setgid rule, got %s", rule.ID)
			}

			if event.SetGID.GID != 1005 {
				t.Errorf("expected gid 1005, got %d", event.SetGID.GID)
			}
		}
	})

	t.Run("setregid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 1006, 1007, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setregid" {
				t.Errorf("expected test_setregid rule, got %s", rule.ID)
			}

			if event.SetGID.GID != 1006 {
				t.Errorf("expected gid 1006, got %d", event.SetGID.GID)
			}
			if event.SetGID.EGID != 1007 {
				t.Errorf("expected egid 1007, got %d", event.SetGID.EGID)
			}
		}
	})

	t.Run("setresgid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETRESGID, 1006, 1007, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setregid" {
				t.Errorf("expected test_setregid rule, got %s", rule.ID)
			}

			if event.SetGID.GID != 1006 {
				t.Errorf("expected gid 1006, got %d", event.SetGID.GID)
			}
			if event.SetGID.EGID != 1007 {
				t.Errorf("expected egid 1007, got %d", event.SetGID.EGID)
			}
		}
	})

	t.Run("setfsgid", func(t *testing.T) {
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETFSGID, 1008, 0, 0); errno != 0 {
				t.Fatal(errno)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_setfsgid" {
				t.Errorf("expected test_setfsgid rule, got %s", rule.ID)
			}

			if event.SetGID.FSGID != 1008 {
				t.Errorf("expected fsgid 1008, got %d", event.SetGID.FSGID)
			}
		}
	})

	t.Run("capset", func(t *testing.T) {
		threadCapabilities := cap.GetProc()
		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if err := threadCapabilities.SetFlag(cap.Permitted, false, cap.SYS_BOOT); err != nil {
				t.Error(err)
			}
			if err := threadCapabilities.SetFlag(cap.Effective, false, cap.SYS_BOOT); err != nil {
				t.Error(err)
			}
			if err := threadCapabilities.SetFlag(cap.Effective, false, cap.WAKE_ALARM); err != nil {
				t.Error(err)
			}
			if err := threadCapabilities.SetProc(); err != nil {
				t.Error(err)
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_capset" {
				t.Errorf("expected test_capset rule, got %s", rule.ID)
			}

			// transform the collected kernel capabilities into a cap.Set
			newSet := cap.NewSet()
			if err := parseCapIntoSet(event.Capset.CapEffective, cap.Effective, newSet); err != nil {
				t.Errorf("failed to parse cap_effective capability: %v", err)
			}
			if err := parseCapIntoSet(event.Capset.CapPermitted, cap.Permitted, newSet); err != nil {
				t.Errorf("failed to parse cap_permitted capability: %v", err)
			}

			if diff, err := threadCapabilities.Compare(newSet); err != nil || diff != 0 {
				t.Errorf("expected following capability set `%s`, got `%s`, error `%v`, diff `%d`", threadCapabilities, newSet, err, diff)
			}
		}
	})

	// test_capset can be somewhat noisy and some events may leak to the next tests (there is a short delay between the
	// reset of the maps and the reload of the rules, we can't move the reset after the reload either, otherwise the
	// ruleset_reload test won't work => we would have reset the channel that contains the reload event).
	// Load a fake new test module to empty the rules and properly cleanup the channels.
	_, _ = newTestModule(nil, nil, testOpts{})
}

func parseCapIntoSet(capabilities uint64, flag cap.Flag, set *cap.Set) error {
	for _, v := range model.KernelCapabilityConstants {
		if v == 0 {
			continue
		}

		if err := set.SetFlag(flag, int(capabilities)&v == v, cap.Value(math.Log2(float64(v)))); err != nil {
			return err
		}
	}
	return nil
}
