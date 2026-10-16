// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func TestUnshareEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_unshare_newnet",
			Expression: `unshare.flags & CLONE_NEWNET > 0 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_unshare_newns",
			Expression: `unshare.flags & CLONE_NEWNS > 0 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_unshare_newuser_newnet",
			Expression: `unshare.flags & CLONE_NEWUSER > 0 && unshare.flags & CLONE_NEWNET > 0 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_clone_newns_exec",
			Expression: `exec.file.name == "syscall_tester" && exec.argv in ["sleep"]`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatalf("Failed to create test module: %v", err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatalf("Failed to load syscall tester: %v", err)
	}

	runUnshare := func(flags int) func() error {
		return func() error {
			cmd := exec.Command(syscallTester, "unshare-flags", strconv.Itoa(flags))
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command: %w", err)
			}
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("command failed: %w", err)
			}
			return nil
		}
	}

	assertUnshare := func(t *testing.T, flags int) func(*rules.Rule, *model.Event) bool {
		t.Helper()

		return func(_ *rules.Rule, event *model.Event) bool {
			assert.Equal(t, "unshare", event.GetType(), "wrong event type")
			assert.Equal(t, uint64(flags), event.Unshare.Flags, "wrong unshare flags")
			assert.Equal(t, int64(0), event.Unshare.SyscallEvent.Retval, "retval should be 0 for success")
			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateUnshareSchema(t, event)
			return true
		}
	}

	t.Run("unshare-newnet", func(t *testing.T) {
		flags := unix.CLONE_NEWNET
		if err := test.GetEventSent(t, runUnshare(flags), assertUnshare(t, flags), time.Second*3, "test_unshare_newnet"); err != nil {
			t.Error(err)
		}
	})

	// the shape the LPE exploits use: a user namespace paired with a network one
	t.Run("unshare-newuser-newnet", func(t *testing.T) {
		// RHEL 7 kernels disable user namespaces, so unshare(CLONE_NEWUSER) never reaches the hook
		checkKernelCompatibility(t, "user namespaces disabled on RHEL7 kernels", func(kv *kernel.Version) bool {
			return kv.IsRH7Kernel()
		})

		flags := unix.CLONE_NEWUSER | unix.CLONE_NEWNET
		if err := test.GetEventSent(t, runUnshare(flags), assertUnshare(t, flags), time.Second*3, "test_unshare_newuser_newnet"); err != nil {
			t.Error(err)
		}
	})

	// CLONE_NEWNS also drives the internal per-mount unshare_mntns events
	t.Run("unshare-newns", func(t *testing.T) {
		flags := unix.CLONE_NEWNS
		if err := test.GetEventSent(t, runUnshare(flags), assertUnshare(t, flags), time.Second*3, "test_unshare_newns"); err != nil {
			t.Error(err)
		}
	})

	t.Run("clone-newns", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("Not applicable to ebpfless")
		}
		parentMntNS, err := utils.NewNSPathFromPid(utils.Getpid(), utils.MntNsType).GetNSID()
		require.NoError(t, err)

		var (
			cmd        *exec.Cmd
			childMntNS uint32
		)
		defer func() {
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}()

		err = test.GetEventSent(t, func() error {
			cmd = exec.Command(syscallTester, "sleep", "2")
			cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWNS}
			if err := cmd.Start(); err != nil {
				return err
			}
			var nsErr error
			childMntNS, nsErr = utils.NewNSPathFromPath(fmt.Sprintf("/proc/%d/ns/mnt", cmd.Process.Pid), utils.MntNsType).GetNSID()
			return nsErr
		}, func(_ *rules.Rule, event *model.Event) bool {
			assert.NotZero(t, event.Exec.Process.MntNS, "child mntns should be resolved")
			assert.NotEqual(t, parentMntNS, event.Exec.Process.MntNS, "child cloned with CLONE_NEWNS should not inherit parent mntns")
			assert.Equal(t, childMntNS, event.Exec.Process.MntNS, "child mntns should match /proc/<pid>/ns/mnt")
			return true
		}, time.Second*3, "test_clone_newns_exec")
		if err != nil {
			t.Error(err)
		}
	})
}
