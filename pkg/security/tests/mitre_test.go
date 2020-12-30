// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

type testCase struct {
	action       func(t *testing.T)
	expectedRule string
}

func TestMitre(t *testing.T) {
	reader := bytes.NewBufferString(config.DefaultPolicy)

	policy, err := rules.LoadPolicy(reader, "default.policy")
	if err != nil {
		t.Fatal(err)
	}

	test, err := newTestModule(policy.Macros, policy.Rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	time.Sleep(time.Second)

	testCases := []testCase{
		{
			action: func(t *testing.T) {
				f, err := os.Open("/etc/shadow")
				if err != nil {
					t.Fatal(err)
				}
				f.Close()
			},
			expectedRule: "credential_accessed",
		},
		{
			action: func(t *testing.T) {
				f, err := os.Open(fmt.Sprintf("/proc/%d/mem", os.Getpid()))
				if err != nil {
					t.Fatal(err)
				}
				f.Close()
			},
			expectedRule: "memory_dump",
		},
		{
			action: func(t *testing.T) {
				f, err := os.Create("/var/log/service.log")
				if err != nil {
					t.Fatal(err)
				}
				f.Close()

				if err := os.Truncate(fmt.Sprintf("/var/log/service.log"), 0); err != nil {
					t.Fatal(err)
				}
			},
			expectedRule: "logs_altered",
		},
		{
			action: func(t *testing.T) {
				if err := os.Remove("/var/log/service.log"); err != nil {
					t.Fatal(err)
				}
			},
			expectedRule: "logs_removed",
		},
		{
			action: func(t *testing.T) {
				f, err := os.Create("/usr/local/bin/pleaseremoveme")
				if err != nil {
					t.Fatal(err)
				}
				f.Close()

				if err := os.Chmod("/usr/local/bin/pleaseremoveme", 0777); err != nil {
					t.Fatal(err)
				}

				os.Remove("/usr/local/bin/pleaseremoveme")

				// wait a bit to ensure that the discarder will be placed before the file delete
				// see the race explanation in the probe_bpf.go in the invalidate event.
				time.Sleep(2 * time.Second)
			},
			expectedRule: "permissions_changed",
		},
		{
			action: func(t *testing.T) {
				os.Mkdir("/lib/modules", 0660)
				f, err := os.Create("/lib/modules/removeme.ko")
				if err != nil {
					t.Fatal(err)
				}
				f.Close()
				os.Remove("/lib/modules/removeme.ko")
			},
			expectedRule: "kernel_module",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("rule %s", tc.expectedRule), func(t *testing.T) {
			tc.action(t)

			timeout := time.After(3 * time.Second)
			for {
				select {
				case event := <-test.events:
					if _, ok := event.Event.(*sprobe.Event); ok {
						if event.rule.ID == tc.expectedRule {
							return
						}
					} else {
						t.Error("invalid event")
					}
				case <-timeout:
					t.Error("timeout")
					return
				}
			}
		})
	}
}
