// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestMMapEvent(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_mmap",
			Expression: `(mmap.protection & (VM_READ|VM_WRITE|VM_EXEC)) == (VM_READ|VM_WRITE|VM_EXEC) && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("mmap", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			data, err := unix.Mmap(0, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC, unix.MAP_SHARED|unix.MAP_ANON)
			if err != nil {
				return fmt.Errorf("couldn't memory segment: %w", err)
			}

			if err := unix.Munmap(data); err != nil {
				return fmt.Errorf("couldn't unmap memory segment: %w", err)
			}
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "mmap", event.GetType(), "wrong event type")
			assert.Equal(t, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC, event.MMap.Protection&(unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC), fmt.Sprintf("wrong protection: %s", model.Protection(event.MMap.Protection)))
			assert.Equal(t, event.Async, false)

			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			assertFieldEqual(t, event, "process.file.path", executable)

			if !validateMMapSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}
