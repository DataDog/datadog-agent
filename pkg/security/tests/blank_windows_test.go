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

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestThatWindowsCanRunATest(t *testing.T) {
	assert.Equal(t, 2, 2)
}

func TestBasicTest(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_basic_rule",
		Expression: `exec.file.name in ["at.exe","schtasks.exe"]`,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	executable := "schtasks.exe"
	test.WaitSignal(t, func() error {
		cmd := exec.Command(executable)
		return cmd.Run()
	}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
		assertFieldEqualCaseInsensitve(t, event, "exec.file.path", `c:\windows\system32\schtasks.exe`, "wrong exec file path")
		assertFieldIsOneOf(t, event, "process.parent.file.name", []string{"testsuite.exe"}, "wrong process parent file name")
	}))

}

func validateProcessContext(tb testing.TB, event *model.Event) {
}

func (tm *testModule) validateExecEvent(tb *testing.T, kind wrapperType, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)
	}
}
