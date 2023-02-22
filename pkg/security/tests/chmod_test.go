// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestChmod(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `chmod.file.path == "{{.Root}}/test-chmod" && chmod.file.destination.rights in [0707, 0717, 0757] && chmod.file.uid == 98 && chmod.file.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint16(applyUmask(fileMode))
	testFile, testFilePtr, err := test.CreateWithOptions("test-chmod", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	t.Run("fchmod", func(t *testing.T) {
		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			f.Close()
			expectedMode = 0o707
		}()

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscall.SYS_FCHMOD, f.Fd(), uintptr(0o707), 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assertRights(t, uint16(event.Chmod.Mode), 0o707)
			assert.Equal(t, getInode(t, testFile), event.Chmod.File.Inode, "wrong inode")
			assertRights(t, event.Chmod.File.Mode, expectedMode, "wrong initial mode")
			assertNearTime(t, event.Chmod.File.MTime)
			assertNearTime(t, event.Chmod.File.CTime)

			value, _ := event.GetFieldValue("async")
			assert.Equal(t, value.(bool), false)

			test.validateChmodSchema(t, event)
		})
	})

	t.Run("fchmodat", func(t *testing.T) {
		defer func() { expectedMode = 0o757 }()

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall6(syscall.SYS_FCHMODAT, 0, uintptr(testFilePtr), uintptr(0o757), 0, 0, 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assertRights(t, uint16(event.Chmod.Mode), 0o757)
			assert.Equal(t, getInode(t, testFile), event.Chmod.File.Inode, "wrong inode")
			assertRights(t, event.Chmod.File.Mode, expectedMode)
			assertNearTime(t, event.Chmod.File.MTime)
			assertNearTime(t, event.Chmod.File.CTime)

			value, _ := event.GetFieldValue("async")
			assert.Equal(t, value.(bool), false)

			test.validateChmodSchema(t, event)
		})
	})

	t.Run("chmod", ifSyscallSupported("SYS_CHMOD", func(t *testing.T, syscallNB uintptr) {
		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(0o717), 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assertRights(t, uint16(event.Chmod.Mode), 0o717, "wrong mode")
			assert.Equal(t, getInode(t, testFile), event.Chmod.File.Inode, "wrong inode")
			assertRights(t, event.Chmod.File.Mode, expectedMode, "wrong initial mode")
			assertNearTime(t, event.Chmod.File.MTime)
			assertNearTime(t, event.Chmod.File.CTime)

			value, _ := event.GetFieldValue("async")
			assert.Equal(t, value.(bool), false)

			test.validateChmodSchema(t, event)
		})
	}))
}
