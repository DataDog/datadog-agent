// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests,386

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestChown(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
	}

	ruleDef2 := &rules.RuleDefinition{
		ID:         "test_rule2",
		Expression: `chown.file.path == "{{.Root}}/test-symlink" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef, ruleDef2}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
	testFile, testFilePtr, err := test.CreateWithOptions("test-chown", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("chown", func(t *testing.T) {
		if _, _, errno := syscall.Syscall(syscall.SYS_CHOWN, uintptr(testFilePtr), 100, 200); errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if user := event.Chown.UID; user != 100 {
				t.Errorf("expected chown user 100, got %d", user)
			}

			if group := event.Chown.GID; group != 200 {
				t.Errorf("expected chown group 200, got %d", group)
			}

			if inode := getInode(t, testFile); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Chown.File.Mode)&expectedMode)
			}

			if event.Chown.File.UID != 98 {
				t.Errorf("expected initial UID %d, got %d", 98, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 99 {
				t.Errorf("expected initial GID %d, got %d", 99, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})

	t.Run("fchown", func(t *testing.T) {
		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		// fchown syscall
		if _, _, errno := syscall.Syscall(syscall.SYS_FCHOWN, f.Fd(), 101, 201); errno != 0 {
			t.Fatal(err)
		}
		defer f.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if user := event.Chown.UID; user != 101 {
				t.Errorf("expected chown user 101, got %d", user)
			}

			if group := event.Chown.GID; group != 201 {
				t.Errorf("expected chown group 201, got %d", group)
			}
			if inode := getInode(t, testFile); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Chown.File.Mode)&expectedMode)
			}

			if event.Chown.File.UID != 100 {
				t.Errorf("expected initial UID %d, got %d", 100, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 200 {
				t.Errorf("expected initial GID %d, got %d", 200, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})

	t.Run("fchownat", func(t *testing.T) {
		if _, _, errno := syscall.Syscall6(syscall.SYS_FCHOWNAT, 0, uintptr(testFilePtr), uintptr(102), uintptr(202), 0x100, 0); errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if user := event.Chown.UID; user != 102 {
				t.Errorf("expected chown user 102, got %d", user)
			}

			if group := event.Chown.GID; group != 202 {
				t.Errorf("expected chown group 202, got %d", group)
			}

			if inode := getInode(t, testFile); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Chown.File.Mode)&expectedMode)
			}

			if event.Chown.File.UID != 101 {
				t.Errorf("expected initial UID %d, got %d", 101, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 201 {
				t.Errorf("expected initial GID %d, got %d", 201, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})

	t.Run("lchown", func(t *testing.T) {
		testSymlink, testSymlinkPtr, err := test.Path("test-symlink")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(testFile, testSymlink); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testSymlink)

		if _, _, errno := syscall.Syscall(syscall.SYS_LCHOWN, uintptr(testSymlinkPtr), uintptr(103), uintptr(203)); errno != 0 {
			t.Fatal(err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if rule.ID != "test_rule2" {
				t.Errorf("expected triggered rule test_rule2, got %s", rule.ID)
			}

			if user := event.Chown.UID; user != 103 {
				t.Errorf("expected chown user 103, got %d", user)
			}

			if group := event.Chown.GID; group != 203 {
				t.Errorf("expected chown group 203, got %d", group)
			}

			if inode := getInode(t, testSymlink); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&0o777 != 0o777 {
				t.Errorf("expected initial mode %d, got %d", 0o777, int(event.Chown.File.Mode))
			}

			if event.Chown.File.UID != 0 {
				t.Errorf("expected initial UID %d, got %d", 0, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 0 {
				t.Errorf("expected initial GID %d, got %d", 0, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})

	t.Run("lchown32", func(t *testing.T) {
		testSymlink, testSymlinkPtr, err := test.Path("test-symlink")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(testFile, testSymlink); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testSymlink)

		if _, _, errno := syscall.Syscall(syscall.SYS_LCHOWN32, uintptr(testSymlinkPtr), uintptr(103), uintptr(203)); errno != 0 {
			t.Fatal(err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if rule.ID != "test_rule2" {
				t.Errorf("expected triggered rule test_rule2, got %s", rule.ID)
			}

			if user := event.Chown.UID; user != 103 {
				t.Errorf("expected chown user 103, got %d", user)
			}

			if group := event.Chown.GID; group != 203 {
				t.Errorf("expected chown group 203, got %d", group)
			}

			if inode := getInode(t, testSymlink); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&0o777 != 0o777 {
				t.Errorf("expected initial mode %d, got %d", 0o777, int(event.Chown.File.Mode))
			}

			if event.Chown.File.UID != 0 {
				t.Errorf("expected initial UID %d, got %d", 0, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 0 {
				t.Errorf("expected initial GID %d, got %d", 0, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})

	t.Run("fchown32", func(t *testing.T) {
		// fchown syscall
		if _, _, errno := syscall.Syscall(syscall.SYS_FCHOWN32, f.Fd(), 101, 201); errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if user := event.Chown.UID; user != 101 {
				t.Errorf("expected chown user 101, got %d", user)
			}

			if group := event.Chown.GID; group != 201 {
				t.Errorf("expected chown group 201, got %d", group)
			}

			if inode := getInode(t, testFile); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Chown.File.Mode)&expectedMode)
			}

			if event.Chown.File.UID != 102 {
				t.Errorf("expected initial UID %d, got %d", 102, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 202 {
				t.Errorf("expected initial GID %d, got %d", 202, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})

	t.Run("chown32", func(t *testing.T) {
		if _, _, errno := syscall.Syscall(syscall.SYS_CHOWN32, uintptr(testFilePtr), 100, 200); errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "chown" {
				t.Errorf("expected chown event, got %s", event.GetType())
			}

			if user := event.Chown.UID; user != 100 {
				t.Errorf("expected chown user 100, got %d", user)
			}

			if group := event.Chown.GID; group != 200 {
				t.Errorf("expected chown group 200, got %d", group)
			}

			if inode := getInode(t, testFile); inode != event.Chown.File.Inode {
				t.Logf("expected inode %d, got %d", event.Chown.File.Inode, inode)
			}

			if int(event.Chown.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Chown.File.Mode)&expectedMode)
			}

			if event.Chown.File.UID != 101 {
				t.Errorf("expected initial UID %d, got %d", 101, event.Chown.File.UID)
			}

			if event.Chown.File.GID != 201 {
				t.Errorf("expected initial GID %d, got %d", 201, event.Chown.File.GID)
			}

			now := time.Now()
			if event.Chown.File.MTime.After(now) || event.Chown.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Chown.File.MTime)
			}

			if event.Chown.File.CTime.After(now) || event.Chown.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Chown.File.CTime)
			}
		}
	})
}
