// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests && amd64

// Package tests holds tests related files
package tests

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestChown32(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "SUSE kernel", func(kv *kernel.Version) bool {
		return kv.IsSuseKernel()
	})

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid in [100, 101, 102, 103, 104, 105, 106] && chown.file.destination.gid in [200, 201, 202, 203, 204, 205, 206]`,
	}

	ruleDef2 := &rules.RuleDefinition{
		ID:         "test_rule2",
		Expression: `chown.file.path == "{{.Root}}/test-symlink" && chown.file.destination.uid in [100, 101, 102, 103, 104, 105, 106] && chown.file.destination.gid in [200, 201, 202, 203, 204, 205, 206]`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef, ruleDef2})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_x86_tester")
	if err != nil {
		t.Fatal(err)
	}

	prevUID := 98
	prevGID := 99
	fileMode := 0o447
	expectedMode := uint32(applyUmask(fileMode))
	testFile, _, err := test.CreateWithOptions("test-chown", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("chown", func(t *testing.T) {

		defer func() {
			prevUID = 100
			prevGID = 200
		}()

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "chown", testFile, "100", "200")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(100), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(200), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("fchown", func(t *testing.T) {
		defer func() {
			prevUID = 101
			prevGID = 201
		}()

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "fchown", testFile, "101", "201")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(101), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(201), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("fchownat", func(t *testing.T) {
		defer func() {
			prevUID = 102
			prevGID = 202
		}()

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "fchownat", testFile, "102", "202")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(102), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(202), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("lchown", func(t *testing.T) {
		testSymlink, _, err := test.Path("test-symlink")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(testFile, testSymlink); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testSymlink)

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "lchown", testSymlink, "103", "203")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(103), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(203), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testSymlink), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(0o777), "wrong initial mode")
			assert.Equal(t, uint32(0), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(0), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("lchown32", func(t *testing.T) {
		testSymlink, _, err := test.Path("test-symlink")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(testFile, testSymlink); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testSymlink)

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "lchown32", testSymlink, "104", "204")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(104), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(204), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testSymlink), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(0o777), "wrong initial mode")
			assert.Equal(t, uint32(0), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(0), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("fchown32", func(t *testing.T) {

		defer func() {
			prevUID = 105
			prevGID = 205
		}()

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "fchown32", testFile, "105", "205")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(105), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(205), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("chown32", func(t *testing.T) {
		defer func() {
			prevUID = 106
			prevGID = 206
		}()

		test.WaitSignal(t, func() error {
			// fchown syscall
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "chown32", testFile, "106", "206")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(106), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(206), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})
}
