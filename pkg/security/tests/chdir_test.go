// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestChdir(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_chdir_rule",
			Expression: `chdir.file.path == "{{.Root}}/test-chdir"`,
		},
		{
			ID:         "test_chdir_ctx",
			Expression: `chdir.file.path == "{{.Root}}/test-chdir-ctx" && chdir.syscall.path == "../../../..{{.Root}}/test-chdir-ctx"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFolder, _, err := test.Path("test-chdir")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(testFolder, 0777); err != nil {
		t.Fatalf("failed to create directory: %s", err)
	}
	defer os.RemoveAll(testFolder)

	t.Run("chdir", func(t *testing.T) {
		SkipIfNotAvailable(t)

		test.WaitSignal(t, func() error {
			return os.Chdir(testFolder)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_chdir_rule")

			validateSyscallContext(t, event, "$.syscall.chdir.path")
		})
	})

	t.Run("fchdir", func(t *testing.T) {
		SkipIfNotAvailable(t)

		test.WaitSignal(t, func() error {
			f, err := os.Open(testFolder)
			if err != nil {
				return err
			}
			defer f.Close()

			return f.Chdir()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_chdir_rule")
		})
	})

	t.Run("syscall-context", func(t *testing.T) {
		SkipIfNotAvailable(t)

		testFolder, _, err := test.Path("test-chdir-ctx")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.MkdirAll(testFolder, 0777); err != nil {
			t.Fatalf("failed to create directory: %s", err)
		}
		defer os.RemoveAll(testFolder)

		test.WaitSignal(t, func() error {
			return os.Chdir("../../../.." + testFolder)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_chdir_ctx")

			validateSyscallContext(t, event, "$.syscall.chdir.path")
		})
	})
}
