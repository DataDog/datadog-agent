// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// nsInode returns the inode number of an nsfs file, which is the namespace ID CWS reports
func nsInode(t *testing.T, path string) uint32 {
	t.Helper()

	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	return uint32(stat.Ino)
}

func TestSetNS(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_setns_netns",
			Expression: `setns.nstype == CLONE_NEWNET && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setns_mntns",
			Expression: `setns.nstype == CLONE_NEWNS && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setns_any",
			Expression: `setns.nstype == 0 && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("join-own-netns", func(t *testing.T) {
		netns := nsInode(t, "/proc/self/ns/net")

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "net")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_netns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNET, event.SetNS.NSType, "wrong namespace type")
			assert.Greater(t, event.SetNS.FD, 0, "the target namespace fd should be valid")
			assert.Equal(t, netns, event.SetNS.NetNS, "should have joined its own network namespace")

			test.validateSetNSSchema(t, event)
		}, "test_setns_netns")
	})

	t.Run("join-own-mntns", func(t *testing.T) {
		mntns := nsInode(t, "/proc/self/ns/mnt")

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "mnt")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_mntns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNS, event.SetNS.NSType, "wrong namespace type")
			assert.Equal(t, mntns, event.SetNS.MntNS, "should have joined its own mount namespace")

			test.validateSetNSSchema(t, event)
		}, "test_setns_mntns")
	})

	// a nstype of 0 lets the kernel infer the namespace type from the file descriptor: the
	// requested type is reported as-is, while the resolved namespace ID is still filled in
	t.Run("infer-nstype", func(t *testing.T) {
		netns := nsInode(t, "/proc/self/ns/net")

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "any")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_any")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, 0, event.SetNS.NSType, "the requested namespace type should be reported as-is")
			assert.Equal(t, netns, event.SetNS.NetNS, "should have joined its own network namespace")

			test.validateSetNSSchema(t, event)
		}, "test_setns_any")
	})

	// the tester leaves its network namespace before joining the original one back through a
	// file descriptor it kept open: the reported netns must be the original one and not the
	// transient unshared one, which proves the ID is resolved and not read from a stale cache
	t.Run("netns-roundtrip", func(t *testing.T) {
		netns := nsInode(t, "/proc/self/ns/net")

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "netns-roundtrip")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_netns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNET, event.SetNS.NSType, "wrong namespace type")
			assert.Equal(t, netns, event.SetNS.NetNS, "should have joined the original network namespace back")

			test.validateSetNSSchema(t, event)
		}, "test_setns_netns")
	})
}
