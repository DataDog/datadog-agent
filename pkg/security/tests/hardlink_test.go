// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"os"
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func runHardlinkTests(t *testing.T, opts testOpts) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_orig",
			Expression: `exec.file.path == "{{.Root}}/orig-touch"`,
		},
		{
			ID:         "test_rule_link",
			Expression: `exec.file.path == "{{.Root}}/my-touch"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// copy touch to make sure it is place on the same fs, hard link constraint
	executable := which(t, "touch")

	testOrigExecutable, _, err := test.Path("orig-touch")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testOrigExecutable)

	if err = copyFile(executable, testOrigExecutable, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("hardlink-creation", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command(testOrigExecutable, "/tmp/test1")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_orig")
		})

		testNewExecutable, _, err := test.Path("my-touch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		err = os.Link(testOrigExecutable, testNewExecutable)
		if err != nil {
			t.Fatal(err)
		}

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testNewExecutable, "/tmp/test2")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link")
		})
	})

	t.Run("hardlink-created", func(t *testing.T) {
		testNewExecutable, _, err := test.Path("my-touch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		err = os.Link(testOrigExecutable, testNewExecutable)
		if err != nil {
			t.Fatal(err)
		}

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testOrigExecutable, "/tmp/test1")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_orig")
		})

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testNewExecutable, "/tmp/test2")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link")
		})
	})
}

func TestHardLinkWithERPC(t *testing.T) {
	runHardlinkTests(t, testOpts{disableMapDentryResolution: true})
}

func TestHardLinkWithMaps(t *testing.T) {
	runHardlinkTests(t, testOpts{disableERPCDentryResolution: true})
}
