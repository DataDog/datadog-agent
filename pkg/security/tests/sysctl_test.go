// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func readSysctlValue(name string) (string, error) {
	data, err := os.ReadFile(path.Join("/proc/sys", name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeSysctlValue(name string, value string) error {
	file, err := os.OpenFile(path.Join("/proc/sys", name), os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(value)
	if err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	return nil
}

func TestSysctlEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	checkKernelCompatibility(t, "missing cgroup/sysctl support", func(kv *kernel.Version) bool {
		mp, err := utils.GetCgroup2MountPoint()
		return !kv.HasCgroupSysctlSupportWithRingbuf() || err != nil || len(mp) == 0
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_sysctl_write",
			Expression: `sysctl.name == "kernel/kptr_restrict" && sysctl.old_value == "1" && sysctl.value == "0" && sysctl.action == SYSCTL_WRITE && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_sysctl_read",
			Expression: `sysctl.name == "kernel/kptr_restrict" && sysctl.value == "0" && sysctl.action == SYSCTL_READ && process.file.name == "testsuite"`,
		},
	}

	// keep the initial value for later
	initialValue, err := readSysctlValue("kernel/kptr_restrict")
	if err != nil {
		t.Fatalf("couldn't read kernel/kptr_restrict: %s", err)
	}
	defer func() {
		// reset initial value
		if err = writeSysctlValue("kernel/kptr_restrict", initialValue); err != nil {
			t.Fatalf("couldn't reset kernel/kptr_restrict to %s: %s", initialValue, err)
		}
	}()

	// make sure the correct value is set before the test starts
	if err := writeSysctlValue("kernel/kptr_restrict", "1"); err != nil {
		t.Fatalf("couldn't set kernel/kptr_restrict: %v", err)
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("test_sysctl_write", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return writeSysctlValue("kernel/kptr_restrict", "0")
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "sysctl", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.SysCtl.FilePosition, "wrong file position")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSysctlSchema(t, event)
		}, "test_sysctl_write")
	})

	t.Run("test_sysctl_read", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			_, err := readSysctlValue("kernel/kptr_restrict")
			return err
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "sysctl", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.SysCtl.FilePosition, "wrong file position")
			assert.Equal(t, "0", event.SysCtl.OldValue, "wrong old value")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSysctlSchema(t, event)
		}, "test_sysctl_read")
	})
}
