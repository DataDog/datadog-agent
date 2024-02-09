// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
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
			ID:         "test_rule_orig_exec",
			Expression: `exec.file.path == "{{.Root}}/orig-touch"`,
		},
		{
			ID:         "test_rule_link_exec",
			Expression: `exec.file.path == "{{.Root}}/my-touch"`,
		},
		{
			ID:         "test_rule_link_creation",
			Expression: `link.file.path == "{{.Root}}/orig-touch" && link.file.destination.path == "{{.Root}}/my-touch"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(opts))
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

	t.Run("exec-orig-then-link-then-exec-link", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command(testOrigExecutable, "/tmp/test1")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_orig_exec")
		})

		testNewExecutable, _, err := test.Path("my-touch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		test.WaitSignal(t, func() error {
			err = os.Link(testOrigExecutable, testNewExecutable)
			if err != nil {
				t.Fatal(err)
			}
			return err
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link_creation")
		})

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testNewExecutable, "/tmp/test2")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link_exec")
		})
	})

	t.Run("link-then-exec-orig-then-exec-link", func(t *testing.T) {
		testNewExecutable, _, err := test.Path("my-touch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		test.WaitSignal(t, func() error {
			err = os.Link(testOrigExecutable, testNewExecutable)
			if err != nil {
				t.Fatal(err)
			}
			return err
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link_creation")
		})

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testOrigExecutable, "/tmp/test1")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_orig_exec")
		})

		test.WaitSignal(t, func() error {
			cmd := exec.Command(testNewExecutable, "/tmp/test2")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link_exec")
		})
	})
}

func TestHardLinkExecsWithERPC(t *testing.T) {
	SkipIfNotAvailable(t)
	runHardlinkTests(t, testOpts{disableMapDentryResolution: true})
}

func TestHardLinkExecsWithMaps(t *testing.T) {
	SkipIfNotAvailable(t)
	runHardlinkTests(t, testOpts{disableERPCDentryResolution: true})
}

func TestHardLink(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_link_creation",
			Expression: `link.file.path == "{{.Root}}/orig-touch" && link.file.destination.path == "{{.Root}}/my-touch"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
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
		testNewExecutable, _, err := test.Path("my-touch")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testNewExecutable)

		test.WaitSignal(t, func() error {
			// nb: this wil test linkat, not link.
			err = os.Link(testOrigExecutable, testNewExecutable)
			if err != nil {
				t.Fatal(err)
			}
			return err
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_link_creation")
		})
	})
}
