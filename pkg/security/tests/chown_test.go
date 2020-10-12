// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests,!386

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
		Expression: `chown.filename == "{{.Root}}/test-chown"`,
	}

	ruleDef2 := &rules.RuleDefinition{
		ID:         "test_rule2",
		Expression: `chown.filename == "{{.Root}}/test-symlink"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef, ruleDef2}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-chown")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)
	defer f.Close()

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
		}
	})

	t.Run("fchown", func(t *testing.T) {
		// fchown syscall
		if _, _, errno := syscall.Syscall(syscall.SYS_FCHOWN, f.Fd(), 101, 201); errno != 0 {
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
		}
	})
}
