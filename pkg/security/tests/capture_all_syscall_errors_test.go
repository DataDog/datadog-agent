// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

// To access the path, we need to use the syscall context instead of the chmod event
var (
	chmodEnoentRule = &rules.RuleDefinition{
		ID:         "test_chmod_capture_enoent",
		Expression: `chmod.syscall.path == "{{.Root}}/does-not-exist" && chmod.retval == ENOENT`,
	}
	openEnoentRule = &rules.RuleDefinition{
		ID:         "test_open_capture_enoent",
		Expression: `open.syscall.path == "{{.Root}}/does-not-exist" && open.retval == ENOENT`,
	}
)

// TestCaptureAllSyscallErrors verifies that with
// runtime_security_config.syscalls.capture_all_errors.enabled set to true,
// chmod() and open() syscalls failing with ENOENT (normally filtered by
// IS_UNHANDLED_ERROR) still produce events in userspace.
func TestCaptureAllSyscallErrors(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		chmodEnoentRule,
		openEnoentRule,
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		captureAllSyscallErrorsEnabled: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(test.Root(), "does-not-exist")

	t.Run("chmod-enoent", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "chmod-error", path)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_chmod_capture_enoent")
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, -int64(syscall.ENOENT), event.Chmod.Retval, "wrong retval")
			// When the file does not exist, the dentry resolution should return an empty string here
			assert.Equal(t, "", event.Chmod.File.PathnameStr)
		}, "test_chmod_capture_enoent")
	})
	t.Run("open-enoent", func(t *testing.T) {
		// On CentOS 7.9, dentry resolution for a missing file returns an empty path instead of
		// the first existing parent, so the event is flagged as Error and the rule never matches.
		checkKernelCompatibility(t, "open ENOENT capture on CentOS 7.9", func(kv *kernel.Version) bool {
			return kv.IsRH7Kernel()
		})

		test.WaitSignalFromRule(t, func() error {
			_, err := os.Open(path)
			if err == nil {
				t.Errorf("should fail with ENOENT: %v", err)
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_open_capture_enoent")
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, -int64(syscall.ENOENT), event.Open.Retval, "wrong retval")
			// When the file does not exist, the dentry resolution should return the first existing parent
			assert.Equal(t, test.Root(), event.Open.File.PathnameStr)
		}, "test_open_capture_enoent")
	})

}

func TestCaptureAllSyscallErrorsDisabledByDefault(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		chmodEnoentRule,
		openEnoentRule,
	}

	// captureAllSyscallErrorsEnabled is intentionally left at its zero value (false)
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(test.Root(), "does-not-exist")

	t.Run("chmod-enoent-dropped", func(t *testing.T) {
		_ = test.GetSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "chmod-error", path)
		}, func(_ *model.Event, rule *rules.Rule) {
			t.Errorf("unexpected event for chmod ENOENT (rule %q): the kernel should have dropped it", rule.ID)
		})
	})

	t.Run("open-enoent-dropped", func(t *testing.T) {
		_ = test.GetSignal(t, func() error {
			_, err := os.Open(path)
			if err == nil {
				t.Errorf("should fail with ENOENT: %v", err)
				return err
			}
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			t.Errorf("unexpected event for open ENOENT (rule %q): the kernel should have dropped it", rule.ID)
		})
	})
}
