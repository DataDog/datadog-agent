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
)

func TestBasicRegistryTest(t *testing.T) {
	openDef := &rules.RuleDefinition{
		ID:         "test_open_rule",
		Expression: `open.registry.key_path == "HKEY_LOCAL_MACHINE\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"`,
	}

	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{openDef}, withStaticOpts(opts))

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(5 * time.Second)

	test.Run(t, "registry 1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		/*
			inputargs := []string{
				"add",
				"HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run",
				"/v",
				"test",
				"/t",
				"REG_SZ",
				"/d",
				"c:\\windows\\system32\\calc.exe",
			}
		*/
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
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("powershell.exe", inputargs, nil)

			// we will ignore any error
			_ = cmd.Run()
			return nil
		}, test.validateRegistryEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "open.registry.key_path", `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, "wrong registry key path")
		}))
	})
}

func (tm *testModule) validateRegistryEvent(tb *testing.T, kind wrapperType, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)
	}
}
