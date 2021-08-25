// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestUnlink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.file.path in ["{{.Root}}/test-unlink", "{{.Root}}/test-unlinkat"] && unlink.file.uid == 98 && unlink.file.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint16(applyUmask(fileMode))
	testFile, testFilePtr, err := test.CreateWithOptions("test-unlink", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	inode := getInode(t, testFile)

	t.Run("unlink", ifSyscallSupported("SYS_UNLINK", func(t *testing.T, syscallNB uintptr) {
		err = test.GetSignal(t, func() error {
			if _, _, err := syscall.Syscall(syscallNB, uintptr(testFilePtr), 0, 0); err != 0 {
				t.Fatal(err)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong inode")
			assertRights(t, event.Unlink.File.Mode, expectedMode)

			assertNearTime(t, event.Unlink.File.MTime)
			assertNearTime(t, event.Unlink.File.CTime)
		})
	}))

	testAtFile, testAtFilePtr, err := test.CreateWithOptions("test-unlinkat", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testAtFile)

	inode = getInode(t, testAtFile)

	t.Run("unlinkat", func(t *testing.T) {
		err = test.GetSignal(t, func() error {
			if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testAtFilePtr), 0); err != 0 {
				t.Fatal(err)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong inode")
			assertRights(t, event.Unlink.File.Mode, expectedMode)

			assertNearTime(t, event.Unlink.File.MTime)
			assertNearTime(t, event.Unlink.File.CTime)
		})
	})
}

func TestUnlinkInvalidate(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.file.path =~ "{{.Root}}/test-unlink-*"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for i := 0; i != 5; i++ {
		filename := fmt.Sprintf("test-unlink-%d", i)

		testFile, _, err := test.Path(filename)
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		err = test.GetSignal(t, func() error {
			os.Remove(testFile)
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "unlink.file.path", testFile)
		})
		if err != nil {
			t.Error(err)
		}
	}
}
