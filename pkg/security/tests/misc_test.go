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
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestEnv(t *testing.T) {
	if testEnvironment != "" && testEnvironment != HostEnvironment && testEnvironment != DockerEnvironment {
		t.Error("invalid environment")
		return
	}
}

func TestOsOrigin(t *testing.T) {
	SkipIfNotAvailable(t)

	origin := "ebpf"
	if ebpfLessEnabled {
		origin = "ebpfless"
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_origin",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-origin" && event.origin == "%s" && event.os == "%s"`, origin, runtime.GOOS),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create("test-origin")
		if err != nil {
			return err
		}
		return os.Remove(testFile)
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_origin")
	})
}

func TestSyscallContext(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDef := &rules.RuleDefinition{
		ID:         "test_chdir_ctx",
		Expression: `chdir.file.path == "{{.Root}}/test-chdir-ctx" && chdir.syscall.str_arg1 == "../../../..{{.Root}}/test-chdir-ctx"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("chdir", func(t *testing.T) {
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
		})
	})
}
