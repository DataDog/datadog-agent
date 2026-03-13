// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestCapabilitiesEvent(t *testing.T) {
	SkipIfNotAvailable(t)
	// skip tests if we are running within a container
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	checkKernelCompatibility(t, "Missing bpf_for_each_map_elem helper", func(kv *kernel.Version) bool {
		return !kv.HasBPFForEachMapElemHelper()
	})

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	checkKernelCompatibility(t, "no override_creds/restore_creds", func(kv *kernel.Version) bool {
		return kv.Code >= kernel.Kernel6_14
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_capabilities_used_exec_flush",
			Expression: `capabilities.used == CAP_SYS_CHROOT && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_capabilities_attempted_exit_flush",
			Expression: `capabilities.attempted == CAP_SYS_PACCT && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_capabilities_used_periodic_flush",
			Expression: `capabilities.used == CAP_CHOWN && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		capabilitiesMonitoringEnabled: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstance, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	t.Run("used-exec-flush", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return dockerInstance.Command(syscallTester, []string{"chroot", "/", ";", "self-exec", "self-exec", "check"}, []string{}).Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "capabilities", event.GetType(), "wrong event type")
			assert.Equal(t, "test_capabilities_used_exec_flush", rule.ID, "wrong rule ID")
			assert.Equal(t, uint64(1<<unix.CAP_SYS_CHROOT), event.CapabilitiesUsage.Attempted, "wrong capabilities attempted")
			assert.Equal(t, uint64(1<<unix.CAP_SYS_CHROOT), event.CapabilitiesUsage.Used, "wrong capabilities used")
			assert.Equal(t, uint64(1<<unix.CAP_SYS_CHROOT), event.ProcessCacheEntry.CapsAttempted&(1<<unix.CAP_SYS_CHROOT), "capabilities attempted should contain CAP_SYS_CHROOT")
			assert.Equal(t, uint64(1<<unix.CAP_SYS_CHROOT), event.ProcessCacheEntry.CapsUsed&(1<<unix.CAP_SYS_CHROOT), "capabilities used should contain CAP_SYS_CHROOT")
		}, "test_capabilities_used_exec_flush")
	})

	t.Run("attempted-exit-flush", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			_ = dockerInstance.Command(syscallTester, []string{"acct"}, []string{}).Run()
			// ignore the error here because the command is expected to fail
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "capabilities", event.GetType(), "wrong event type")
			assert.Equal(t, "test_capabilities_attempted_exit_flush", rule.ID, "wrong rule ID")
			assert.Equal(t, uint64(1<<unix.CAP_SYS_PACCT), event.CapabilitiesUsage.Attempted, "wrong capabilities attempted")
			assert.Equal(t, uint64(0), event.CapabilitiesUsage.Used, "wrong capabilities used")
			assert.Equal(t, uint64(1<<unix.CAP_SYS_PACCT), event.ProcessCacheEntry.CapsAttempted&(1<<unix.CAP_SYS_PACCT), "capabilities attempted should contain CAP_SYS_PACCT")
			assert.Equal(t, uint64(0), event.ProcessCacheEntry.CapsUsed&(1<<unix.CAP_SYS_PACCT), "capabilities used shouldn't contain CAP_SYS_PACCT")
		}, "test_capabilities_attempted_exit_flush")
	})

	t.Run("used-periodic-flush", func(t *testing.T) {
		var syscallTesterCmd *exec.Cmd
		defer func() {
			if syscallTesterCmd != nil {
				if syscallTesterCmd.Process != nil {
					_ = syscallTesterCmd.Process.Kill()
				}
				if err := syscallTesterCmd.Wait(); err != nil {
					t.Logf("syscall_tester command terminated: %v", err)
				}
			}
		}()
		test.WaitSignalFromRule(t, func() error {
			syscallTesterCmd = dockerInstance.Command(syscallTester, []string{"chown", "/etc/profile", "1001", "1001", ";", "pause"}, []string{})
			return syscallTesterCmd.Start()
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "capabilities", event.GetType(), "wrong event type")
			assert.Equal(t, "test_capabilities_used_periodic_flush", rule.ID, "wrong rule ID")
			assert.Equal(t, uint64(1<<unix.CAP_CHOWN), event.CapabilitiesUsage.Attempted, "wrong capabilities attempted")
			assert.Equal(t, uint64(1<<unix.CAP_CHOWN), event.CapabilitiesUsage.Used, "wrong capabilities used")
			assert.Equal(t, uint64(1<<unix.CAP_CHOWN), event.ProcessCacheEntry.CapsAttempted&(1<<unix.CAP_CHOWN), "capabilities attempted should contain CAP_CHOWN")
			assert.Equal(t, uint64(1<<unix.CAP_CHOWN), event.ProcessCacheEntry.CapsUsed&(1<<unix.CAP_CHOWN), "capabilities used should contain CAP_CHOWN")
		}, "test_capabilities_used_periodic_flush")
	})
}
