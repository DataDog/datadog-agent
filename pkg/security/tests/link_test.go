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

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestLink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `link.file.path == "{{.Root}}/test-link" && link.file.destination.path == "{{.Root}}/test2-link" && link.file.uid == 98 && link.file.gid == 99 && link.file.destination.uid == 98 && link.file.destination.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
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

	t.Run("link", ifSyscallSupported("SYS_LINK", func(t *testing.T, syscallNB uintptr) {
		err = test.GetSignal(t, func() error {
			_, _, errno := syscall.Syscall(syscallNB, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
			if errno != 0 {
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "link", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Link.Source.Inode, "wrong inode")

			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))

			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)
		})
		if err != nil {
			t.Error(err)
		}

		if err := os.Remove(testNewFile); err != nil {
			t.Error(err)
		}
	}))

	t.Run("linkat", func(t *testing.T) {
		err = test.GetSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_LINKAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
			if errno != 0 {
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "link", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Link.Source.Inode, "wrong inode")

			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))

			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)
		})
		if err != nil {
			t.Error(err)
		}
	})
}
