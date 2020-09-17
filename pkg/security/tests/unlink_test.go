// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestUnlink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.filename == "{{.Root}}/test-unlink" || unlink.filename == "{{.Root}}/testat-unlink" || unlink.filename == "{{.Root}}/testat-rmdir"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-unlink")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(testFile)

	t.Run("unlink", func(t *testing.T) {
		if _, _, err := syscall.Syscall(syscall.SYS_UNLINK, uintptr(testFilePtr), 0, 0); err != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "unlink" {
				t.Errorf("expected unlink event, got %s", event.GetType())
			}
		}
	})

	testatFile, testatFilePtr, err := test.Path("testat-unlink")
	if err != nil {
		t.Fatal(err)
	}

	f, err = os.Create(testatFile)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(testatFile)

	t.Run("unlinkat", func(t *testing.T) {
		if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testatFilePtr), 0); err != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "unlink" {
				t.Errorf("expected unlink event, got %s", event.GetType())
			}
		}
	})

	t.Run("unlinkat-at-removedir", func(t *testing.T) {
		testDir, testDirPtr, err := test.Path("testat-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testDir, 0777); err != nil {
			t.Fatal(err)
		}

		if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testDirPtr), 512); err != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "unlink" {
				t.Errorf("expected unlink event, got %s", event.GetType())
			}
		}
	})
}
