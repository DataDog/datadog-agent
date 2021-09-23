// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestHardLink(t *testing.T) {
	executable := which("touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_orig",
			Expression: fmt.Sprintf(`exec.file.path == "%s"`, executable),
		},
		{
			ID:         "test_rule_link",
			Expression: `exec.file.path == "{{.Root}}/mytouch"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("hardlink-creation", ifSyscallSupported("SYS_LINK", func(t *testing.T, syscallNB uintptr) {
		err = test.GetSignal(t, func() error {
			cmd := exec.Command(executable, "/tmp/test1")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_orig")
		})
		if err != nil {
			t.Error(err)
		}

		testNewExecutable, _, err := test.Path("mytouch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		err = os.Link(executable, testNewExecutable)
		if err != nil {
			t.Fatal(err)
		}

		err = test.GetSignal(t, func() error {
			cmd := exec.Command(testNewExecutable, "/tmp/test2")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link")
		})
		if err != nil {
			t.Error(err)
		}
	}))

	t.Run("hardlink-created", ifSyscallSupported("SYS_LINK", func(t *testing.T, syscallNB uintptr) {
		testNewExecutable, _, err := test.Path("mytouch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		err = os.Link(executable, testNewExecutable)
		if err != nil {
			t.Fatal(err)
		}

		err = test.GetSignal(t, func() error {
			cmd := exec.Command(executable, "/tmp/test1")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_orig")
		})
		if err != nil {
			t.Error(err)
		}

		err = test.GetSignal(t, func() error {
			cmd := exec.Command(testNewExecutable, "/tmp/test2")
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link")
		})
		if err != nil {
			t.Error(err)
		}
	}))
}
