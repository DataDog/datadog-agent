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
			ID:         "test_sysctl",
			Expression: `sysctl.name == "kernel/kptr_restrict" && sysctl.current_value == "0" && sysctl.new_value == "0" && sysctl.action == SYSCTL_WRITE && process.file.name == "testsuite"`,
		},
	}

	// make sure the correct value is set before the test starts
	if err := writeSysctlValue("kernel/kptr_restrict", "0"); err != nil {
		t.Fatalf("couldn't set kernel/kptr_restrict: %v", err)
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("test_sysctl", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return writeSysctlValue("kernel/kptr_restrict", "0")
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "sysctl", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(0), event.SysCtl.FilePosition, "wrong file position")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSysctlSchema(t, event)
		})
	})
}
