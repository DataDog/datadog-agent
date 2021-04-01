// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestMkdir(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdir",
			Expression: `mkdir.file.path == "{{.Root}}/test-mkdir" && mkdir.file.uid == 0 && mkdir.file.gid == 0`,
		},
		{
			ID:         "test_rule_mkdirat",
			Expression: `mkdir.file.path == "{{.Root}}/testat-mkdir" && mkdir.file.uid == 0 && mkdir.file.gid == 0`,
		},
	}

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mkdirMode := 0o707
	expectedMode := applyUmask(mkdirMode)

	t.Run("mkdir", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-mkdir")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, errno := syscall.Syscall(syscall.SYS_MKDIR, uintptr(testFilePtr), uintptr(mkdirMode), 0); errno != 0 {
			t.Fatal(err)
		}
		defer syscall.Rmdir(testFile)

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_rule_mkdir" {
				t.Errorf("expected triggered rule 'test_rule_mkdir', got '%s'", rule.ID)
			}

			if mode := event.Mkdir.Mode; mode != uint32(mkdirMode) {
				t.Errorf("expected mkdir mode %d, got %#o (%+v)", mkdirMode, mode, event)
			}

			if inode := getInode(t, testFile); inode != event.Mkdir.File.Inode {
				t.Logf("expected inode %d, got %d", event.Mkdir.File.Inode, inode)
			}

			if int(event.Mkdir.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Mkdir.File.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Mkdir.File.MTime.After(now) || event.Mkdir.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Mkdir.File.MTime)
			}

			if event.Mkdir.File.CTime.After(now) || event.Mkdir.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Mkdir.File.CTime)
			}

			testContainerPath(t, event, "mkdir.file.container_path")
		}
	})

	t.Run("mkdirat", func(t *testing.T) {
		testatFile, testatFilePtr, err := test.Path("testat-mkdir")
		if err != nil {
			t.Fatal(err)
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno != 0 {
			t.Fatal(error(errno))
		}
		defer syscall.Rmdir(testatFile)

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_rule_mkdirat" {
				t.Errorf("expected triggered rule 'test_rule_mkdirat', got '%s'", rule.ID)
			}

			if mode := event.Mkdir.Mode; mode != 0777 {
				t.Errorf("expected mkdir mode 0777, got %#o", mode)
			}

			if inode := getInode(t, testatFile); inode != event.Mkdir.File.Inode {
				t.Logf("expected inode %d, got %d", event.Mkdir.File.Inode, inode)
			}

			if int(event.Mkdir.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Mkdir.File.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Mkdir.File.MTime.After(now) || event.Mkdir.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Mkdir.File.MTime)
			}

			if event.Mkdir.File.CTime.After(now) || event.Mkdir.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Mkdir.File.CTime)
			}

			testContainerPath(t, event, "mkdir.file.container_path")
		}
	})
}

func TestMkdirError(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdirat_error",
			Expression: `process.file.name == "{{.ProcessName}}" && mkdir.retval == EACCES`,
		},
	}

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("mkdirat-error", func(t *testing.T) {
		_, testatFilePtr, err := test.Path("testat2-mkdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chmod(test.Root(), 0711); err != nil {
			t.Fatal(err)
		}

		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 10000, 10000, 0); errno != 0 {
				t.Fatal(err)
			}

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 10000, 10000, 0); errno != 0 {
				t.Fatal(err)
			}

			if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno == 0 {
				t.Fatal(error(errno))
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if rule.ID != "test_rule_mkdirat_error" {
				t.Errorf("expected triggered rule 'test_rule_mkdirat_error', got '%s'", rule.ID)
			}

			if retval := event.Mkdir.Retval; retval != -int64(syscall.EACCES) {
				t.Errorf("expected retval != EACCES, got %d", retval)
			}
		}
	})
}
