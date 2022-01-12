// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

func TestMProtectEvent(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_mprotect",
			Expression: `(mprotect.vm_protection & VM_WRITE > 0) && (mprotect.req_protection & VM_EXEC > 0)`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("mprotect", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var data []byte
			data, err = unix.Mmap(0, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANON)
			if err != nil {
				return fmt.Errorf("couldn't memory segment: %v", err)
			}

			if err = unix.Mprotect(data, unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC); err != nil {
				return fmt.Errorf("couldn't mprotect segment: %v", err)
			}
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "mprotect", event.GetType(), "wrong event type")
			assert.NotEqual(t, 0, event.MProtect.VMProtection&(unix.PROT_READ|unix.PROT_WRITE), fmt.Sprintf("wrong initial protection: %s", model.Protection(event.MProtect.VMProtection)))
			assert.NotEqual(t, 0, event.MProtect.ReqProtection&(unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC), fmt.Sprintf("wrong requested protection: %s", model.Protection(event.MProtect.ReqProtection)))

			if !validateMProtectSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}
