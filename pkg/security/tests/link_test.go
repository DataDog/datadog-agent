// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestLink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `link.file.path == "{{.Root}}/test-link" && link.file.destination.path == "{{.Root}}/test2-link" && link.file.uid == 98 && link.file.gid == 99 && link.file.destination.uid == 98 && link.file.destination.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
	_, testOldFilePtr, err := test.CreateWithOptions("test-link", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	testNewFile, testNewFilePtr, err := test.Path("test2-link")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("link", func(t *testing.T) {
		_, _, errno := syscall.Syscall(syscall.SYS_LINK, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
		if errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "link" {
				t.Errorf("expected link event, got %s", event.GetType())
			}

			testContainerPath(t, event, "link.file.container_path")
			testContainerPath(t, event, "link.file.destination.container_path")

			if int(event.Link.Source.Mode) & expectedMode != expectedMode {
				t.Errorf("expected source mode %d, got %d", expectedMode, event.Link.Source.Mode)
			}

			if int(event.Link.Target.Mode) & expectedMode != expectedMode {
				t.Errorf("expected target mode %d, got %d", expectedMode, event.Link.Target.Mode)
			}

			now := time.Now()
			if event.Link.Source.MTime.After(now) || event.Link.Source.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected source mtime close to %s, got %s", now, event.Link.Source.MTime)
			}

			if event.Link.Source.CTime.After(now) || event.Link.Source.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected source ctime close to %s, got %s", now, event.Link.Source.CTime)
			}

			if event.Link.Target.MTime.After(now) || event.Link.Target.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected target mtime close to %s, got %s", now, event.Link.Target.MTime)
			}

			if event.Link.Target.CTime.After(now) || event.Link.Target.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected target ctime close to %s, got %s", now, event.Link.Target.CTime)
			}
		}
	})

	if err := os.Remove(testNewFile); err != nil {
		t.Fatal(err)
	}

	t.Run("linkat", func(t *testing.T) {
		_, _, errno := syscall.Syscall6(syscall.SYS_LINKAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
		if errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "link" {
				t.Errorf("expected rename event, got %s", event.GetType())
			}

			if inode := getInode(t, testNewFile); inode != event.Link.Source.Inode {
				t.Logf("expected inode %d, got %d", event.Link.Source.Inode, inode)
			}

			testContainerPath(t, event, "link.file.container_path")
			testContainerPath(t, event, "link.file.destination.container_path")

			if int(event.Link.Source.Mode) & expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, event.Link.Source.Mode)
			}

			if int(event.Link.Target.Mode) & expectedMode != expectedMode {
				t.Errorf("expected target mode %d, got %d", expectedMode, event.Link.Target.Mode)
			}

			now := time.Now()
			if event.Link.Source.MTime.After(now) || event.Link.Source.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected source mtime close to %s, got %s", now, event.Link.Source.MTime)
			}

			if event.Link.Source.CTime.After(now) || event.Link.Source.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected source ctime close to %s, got %s", now, event.Link.Source.CTime)
			}

			if event.Link.Target.MTime.After(now) || event.Link.Target.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected target mtime close to %s, got %s", now, event.Link.Target.MTime)
			}

			if event.Link.Target.CTime.After(now) || event.Link.Target.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected target ctime close to %s, got %s", now, event.Link.Target.CTime)
			}
		}
	})
}
