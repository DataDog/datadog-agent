// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

/*
 * implemented as three separate tests instead of single test with subtests (t.run)
 * Originally implemented as subtests, but the tests would intermittently fail
 * because of the cws runtime potentially generating multiple events.  Split into
 * individual tests so that the underlying event system is starting fresh each time
 */

func TestBasicRegistryTestPowershell(t *testing.T) {
	openDef := &rules.RuleDefinition{
		ID:         "test_open_rule",
		Expression: `open.registry.key_path == "HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Run"`,
	}
	createDef := &rules.RuleDefinition{
		ID:         "test_create_rule",
		Expression: `create.registry.key_path == "HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Run"`,
	}

	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{openDef, createDef}, withStaticOpts(opts))

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// Wait for ETW to be ready (signaled on first event received)
	if !test.WaitForETWReady(30 * time.Second) {
		t.Fatal("Timeout waiting for ETW to be ready")
	}

	test.RunMultiMode(t, "Test registry with powershell", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		inputargs := []string{
			"-c",
			"Set-ItemProperty",
			"-Path",
			`"HKLM:\Software\Microsoft\Windows\CurrentVersion\Run"`,
			"-Name",
			`"test"`,
			"-Value",
			`"test"`,
		}
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("powershell.exe", inputargs, nil)

			// we will ignore any error
			_ = cmd.Run()
			return nil
		}, test.validateRegistryEvent(t, noWrapperType, func(event *model.Event, _ *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "open.registry.key_path", `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "wrong registry key path")
		}), "test_open_rule")
	})
}

func TestBasicRegistryTestRegExe(t *testing.T) {
	openDef := &rules.RuleDefinition{
		ID:         "test_open_rule",
		Expression: `open.registry.key_path == "HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Run"`,
	}
	createDef := &rules.RuleDefinition{
		ID:         "test_create_rule",
		Expression: `create.registry.key_path == "HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Run"`,
	}

	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{openDef, createDef}, withStaticOpts(opts))

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// Wait for ETW to be ready (signaled on first event received)
	if !test.WaitForETWReady(30 * time.Second) {
		t.Fatal("Timeout waiting for ETW to be ready")
	}

	test.RunMultiMode(t, "Test registry with reg.exe", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		inputargs := []string{
			"add",
			"HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run",
			"/f",
			"/v",
			"test",
			"/t",
			"REG_SZ",
			"/d",
			"c:\\windows\\system32\\calc.exe",
		}
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc("reg.exe", inputargs, nil)

			// we will ignore any error
			_ = cmd.Run()
			return nil
		}, test.validateRegistryEvent(t, noWrapperType, func(event *model.Event, _ *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "create.registry.key_path", `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "wrong registry key path")
		}), "test_create_rule")
	})
}

func TestBasicRegistryTestAPI(t *testing.T) {
	openDef := &rules.RuleDefinition{
		ID:         "test_open_rule",
		Expression: `open.registry.key_path == "HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Run"`,
	}
	createDef := &rules.RuleDefinition{
		ID:         "test_create_rule",
		Expression: `create.registry.key_path == "HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Run"`,
	}

	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{openDef, createDef}, withStaticOpts(opts))

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// Wait for ETW to be ready (signaled on first event received)
	if !test.WaitForETWReady(30 * time.Second) {
		t.Fatal("Timeout waiting for ETW to be ready")
	}

	test.RunMultiMode(t, "Test registry with API", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			key, _, err := registry.CreateKey(windows.HKEY_LOCAL_MACHINE, `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, windows.KEY_READ|windows.KEY_WRITE)
			if err == nil {
				defer key.Close()
			}
			return nil

		}, test.validateRegistryEvent(t, noWrapperType, func(event *model.Event, _ *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "create.registry.key_path", `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "wrong registry key path")
		}), "test_create_rule")
	})
}

func (tm *testModule) validateRegistryEvent(tb *testing.T, kind wrapperType, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)
	}
}
