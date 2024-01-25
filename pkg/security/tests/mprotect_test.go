// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestMProtectEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_mprotect",
			Expression: `(mprotect.vm_protection & (VM_READ|VM_WRITE)) == (VM_READ|VM_WRITE) && (mprotect.req_protection & (VM_READ|VM_WRITE|VM_EXEC)) == (VM_READ|VM_WRITE|VM_EXEC) && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("mprotect", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var data []byte
			data, err = unix.Mmap(0, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANON)
			if err != nil {
				return fmt.Errorf("couldn't memory segment: %w", err)
			}

			if err = unix.Mprotect(data, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
				return fmt.Errorf("couldn't mprotect segment: %w", err)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "mprotect", event.GetType(), "wrong event type")
			assert.Equal(t, unix.PROT_READ|unix.PROT_WRITE, event.MProtect.VMProtection&(unix.PROT_READ|unix.PROT_WRITE), fmt.Sprintf("wrong initial protection: %s", model.Protection(event.MProtect.VMProtection)))
			assert.Equal(t, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC, event.MProtect.ReqProtection&(unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC), fmt.Sprintf("wrong requested protection: %s", model.Protection(event.MProtect.ReqProtection)))

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			assertFieldEqual(t, event, "process.file.path", executable)

			test.validateMProtectSchema(t, event)
		})
	})
}
