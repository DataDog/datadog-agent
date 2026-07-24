// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// TestPIDFDGetfd checks that stealing a file descriptor out of another process
// with pidfd_getfd(2) is reported as an `open` event on the resolved path,
// attributed to the thief (the process that called pidfd_getfd).
//
// We spawn a small shell command as the victim: it opens our target file
// read-write on a known fd and holds it open. The test process is the thief, so
// it knows the victim's pid (the command's pid) and the fd to steal (the one we
// told the shell to use). The rule is scoped to our own pid, so the victim's
// setup open is excluded and the only event that can match is the synthetic
// `open` we expect the pidfd_getfd hook to emit for the thief.
func TestPIDFDGetfd(t *testing.T) {
	SkipIfNotAvailable(t)

	// pidfd_getfd(2) was introduced in Linux 5.6.
	checkKernelCompatibility(t, "pidfd_getfd requires kernel 5.6+", func(kv *kernel.Version) bool {
		return kv.Code < kernel.Kernel5_6
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_pidfd_getfd",
			Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-pidfd-getfd" && process.pid == %d && event.is_pidfd`, os.Getpid()),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-pidfd-getfd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	const victimFd = 3

	test.WaitSignalFromRule(t, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		victim := exec.CommandContext(ctx, "/bin/sh", "-c",
			fmt.Sprintf("exec %d<>'%s'; echo ready; exec sleep 30", victimFd, testFile))
		stdout, err := victim.StdoutPipe()
		if err != nil {
			return err
		}
		if err := victim.Start(); err != nil {
			return err
		}
		defer func() {
			_ = victim.Process.Kill()
			_ = victim.Wait()
		}()

		buf := make([]byte, len("ready\n"))
		if _, err := io.ReadFull(stdout, buf); err != nil {
			return fmt.Errorf("victim did not become ready: %w", err)
		}

		pidfd, err := unix.PidfdOpen(victim.Process.Pid, 0)
		if err != nil {
			return fmt.Errorf("pidfd_open(%d): %w", victim.Process.Pid, err)
		}
		defer unix.Close(pidfd)

		stolenFd, err := unix.PidfdGetfd(pidfd, victimFd, 0)
		if err != nil {
			return fmt.Errorf("pidfd_getfd(%d): %w", victimFd, err)
		}
		unix.Close(stolenFd)
		return nil
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
		assert.Equal(t, unix.O_RDWR, int(event.Open.Flags)&unix.O_ACCMODE, "wrong flags")
		assertInode(t, event.Open.File.Inode, getInode(t, testFile))
		assert.NotZero(t, event.Flags&model.EventFlagsPIDFD, "event should be flagged as pidfd-originated")

		// The syscall context holds the actual pidfd_getfd arguments
		// (pidfd, targetfd, flags), not the open layout (path, flags, mode),
		// so they are surfaced through the int1/int2/int3 slots.
		sc := &event.Open.SyscallContext
		assert.Positive(t, event.FieldHandlers.ResolveSyscallCtxArgsInt1(event, sc), "pidfd should be a valid fd")
		assert.Equal(t, victimFd, event.FieldHandlers.ResolveSyscallCtxArgsInt2(event, sc), "wrong targetfd")
		assert.Equal(t, 0, event.FieldHandlers.ResolveSyscallCtxArgsInt3(event, sc), "wrong pidfd_getfd flags")
	}, "test_pidfd_getfd")
}

// TestPIDFDSendSignal checks whether sending a signal through pidfd_send_signal(2)
// is reported as a `signal` event.
func TestPIDFDSendSignal(t *testing.T) {
	SkipIfNotAvailable(t)

	// pidfd_open(2), used to obtain the pidfd, was introduced in Linux 5.3.
	checkKernelCompatibility(t, "pidfd_send_signal requires kernel 5.3+", func(kv *kernel.Version) bool {
		return kv.Code < kernel.Kernel5_3
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			// scope to the sender (this test process); signal.target is the victim.
			ID:         "test_pidfd_send_signal",
			Expression: fmt.Sprintf(`signal.type == SIGUSR1 && process.pid == %d && event.is_pidfd`, os.Getpid()),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The target: a sleep process we signal through its pidfd.
	victim := exec.CommandContext(ctx, "/bin/sleep", "30")
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = victim.Process.Kill()
		_ = victim.Wait()
	}()
	victimPid := victim.Process.Pid

	test.WaitSignalFromRule(t, func() error {
		pidfd, err := unix.PidfdOpen(victimPid, 0)
		if err != nil {
			return fmt.Errorf("pidfd_open(%d): %w", victimPid, err)
		}
		defer unix.Close(pidfd)

		if err := unix.PidfdSendSignal(pidfd, unix.SIGUSR1, nil, 0); err != nil {
			return fmt.Errorf("pidfd_send_signal: %w", err)
		}
		return nil
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "signal", event.GetType(), "wrong event type")
		assert.Equal(t, uint32(unix.SIGUSR1), event.Signal.Type, "wrong signal")
		assert.Equal(t, uint32(victimPid), event.Signal.PID, "wrong target pid")
		assert.NotZero(t, event.Flags&model.EventFlagsPIDFD, "event should be flagged as pidfd-originated")
	}, "test_pidfd_send_signal")
}
