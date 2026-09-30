// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// nsInode returns the inode number of an nsfs file, which is the namespace ID CWS reports, or 0
// when the namespace type doesn't exist on this kernel
func nsInode(t *testing.T, path string) uint32 {
	t.Helper()

	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	return uint32(stat.Ino)
}

// ownNamespaceIDs returns the namespace IDs of the test process, which the syscall tester inherits
func ownNamespaceIDs(t *testing.T) model.NamespaceIDs {
	t.Helper()

	return model.NamespaceIDs{
		MntNS:    nsInode(t, "/proc/self/ns/mnt"),
		NetNS:    nsInode(t, "/proc/self/ns/net"),
		PIDNS:    nsInode(t, "/proc/self/ns/pid_for_children"),
		UserNS:   nsInode(t, "/proc/self/ns/user"),
		UTSNS:    nsInode(t, "/proc/self/ns/uts"),
		IPCNS:    nsInode(t, "/proc/self/ns/ipc"),
		CgroupNS: nsInode(t, "/proc/self/ns/cgroup"),
		TimeNS:   nsInode(t, "/proc/self/ns/time"),
	}
}

func TestSetNS(t *testing.T) {
	SkipIfNotAvailable(t)

	// nstype carries the namespace types the kernel installed, not the ones the caller asked for,
	// so a single rule per type covers every way of requesting it
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_setns_netns",
			Expression: `setns.nstype == CLONE_NEWNET && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setns_mntns",
			Expression: `setns.nstype == CLONE_NEWNS && process.file.name == "syscall_tester"`,
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
		own := ownNamespaceIDs(t)

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "net")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_netns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNET, event.SetNS.NSType, "wrong namespace type")
			assert.Equal(t, own, event.SetNS.NamespaceIDs, "joining its own namespace shouldn't change any namespace")
			assert.Equal(t, own, event.SetNS.Previous, "wrong namespace IDs before the syscall")

			test.validateSetNSSchema(t, event)
		}, "test_setns_netns")
	})

	t.Run("join-own-mntns", func(t *testing.T) {
		own := ownNamespaceIDs(t)

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "mnt")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_mntns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNS, event.SetNS.NSType, "wrong namespace type")
			assert.Equal(t, own, event.SetNS.NamespaceIDs, "joining its own namespace shouldn't change any namespace")
			assert.Equal(t, own, event.SetNS.Previous, "wrong namespace IDs before the syscall")

			test.validateSetNSSchema(t, event)
		}, "test_setns_mntns")
	})

	// A nstype of 0 leaves the kernel to resolve the type from the file descriptor. Reporting the
	// requested value would make this call invisible to a rule on nstype, so that passing 0 instead
	// of the flag would be a one-character evasion. The type the kernel installed is reported
	// instead, so the very same test_setns_netns rule has to match here too.
	t.Run("infer-nstype-from-fd", func(t *testing.T) {
		own := ownNamespaceIDs(t)

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "any")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_netns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNET, event.SetNS.NSType, "the type should be resolved from the fd, not reported as the requested 0")
			assert.Equal(t, own, event.SetNS.NamespaceIDs, "joining its own namespace shouldn't change any namespace")
			assert.Equal(t, own, event.SetNS.Previous, "wrong namespace IDs before the syscall")

			test.validateSetNSSchema(t, event)
		}, "test_setns_netns")
	})

	// the tester leaves its network namespace before joining the original one back through a
	// file descriptor it kept open: the IDs before the syscall must carry the transient unshared
	// netns and the IDs after it the original one, with every other namespace left untouched
	t.Run("netns-roundtrip", func(t *testing.T) {
		own := ownNamespaceIDs(t)

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setns", "netns-roundtrip")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_setns_netns")
			assert.Equal(t, "setns", event.GetType(), "wrong event type")
			assert.Equal(t, int64(0), event.SetNS.Retval, "setns should have succeeded")
			assert.Equal(t, unix.CLONE_NEWNET, event.SetNS.NSType, "wrong namespace type")
			assert.Equal(t, own, event.SetNS.NamespaceIDs, "should have joined the original network namespace back")

			previous := event.SetNS.Previous
			assert.NotZero(t, previous.NetNS, "the unshared network namespace should be resolved")
			assert.NotEqual(t, own.NetNS, previous.NetNS, "the syscall was made from the unshared network namespace")
			previous.NetNS = own.NetNS
			assert.Equal(t, own, previous, "only the network namespace should differ before the syscall")

			test.validateSetNSSchema(t, event)
		}, "test_setns_netns")
	})
}
