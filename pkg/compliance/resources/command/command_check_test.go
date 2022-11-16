// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	resource_test "github.com/DataDog/datadog-agent/pkg/compliance/resources/tests"
	commandutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/command"

	assert "github.com/stretchr/testify/require"
)

var commandModule = `package datadog
			
import data.datadog as dd
import data.helpers as h

compliant(command) {
	%s
}

findings[f] {
	not h.has_key(input, "command")
	f := dd.error_finding(
			"command",
			"",
			sprintf("Binary command 'myCommand' execution failed, error: %%s", [input.context.input.command.command.binary.name]),
		)
}

findings[f] {
	count(input.command) == 0
	f := dd.failing_finding(
			"command",
			"",
			null,
	)
}

findings[f] {
		compliant(input.command)
		f := dd.passed_finding(
				"command",
				"myCommand",
				{ "command.exitCode": input.command.exitCode }
		)
}

findings[f] {
		not compliant(input.command)
		f := dd.failing_finding(
				"command",
				"myCommand",
				{ "command.exitCode": input.command.exitCode }
		)
}`

type commandFixture struct {
	name string

	resource compliance.RegoInput
	module   string

	commandExitCode int
	commandOutput   string
	commandError    error

	expectCommandName string
	expectCommandArgs []string

	expectReport *compliance.Report
}

func (f *commandFixture) mockRunCommand(t *testing.T) commandutils.RunnerFunc {
	return func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
		assert.Equal(t, f.expectCommandName, name)
		assert.Equal(t, f.expectCommandArgs, args)
		return f.commandExitCode, []byte(f.commandOutput), f.commandError
	}
}

func (f *commandFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	commandutils.Runner = f.mockRunCommand(t)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()

	regoRule := resource_test.NewTestRule(f.resource, "command", f.module)

	commandCheck := rego.NewCheck(regoRule)
	err := commandCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := commandCheck.Check(env)
	reportsJSON, _ := json.MarshalIndent(reports, "", "  ")
	t.Log(string(reportsJSON))

	assert.NotEmpty(reports)

	assert.Equal(f.expectReport.Passed, reports[0].Passed)
	assert.Equal(f.expectReport.Data, reports[0].Data)
	if f.expectReport.Error != nil {
		assert.EqualError(f.expectReport.Error, reports[0].Error.Error())
	}
}

func TestCommandCheck(t *testing.T) {
	tests := []commandFixture{
		{
			name: "binary run",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Command: &compliance.Command{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "myCommand",
							Args: []string{"--foo=bar", "--baz"},
						},
					},
				},
				// Condition: `command.stdout == "output"`,
			},
			module:            fmt.Sprintf(commandModule, `input.command.stdout == "output"`),
			commandExitCode:   0,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: "myCommand",
			expectCommandArgs: []string{"--foo=bar", "--baz"},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": json.Number("0"),
				},
			},
		},
		{
			name: "shell run",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Command: &compliance.Command{
						ShellCmd: &compliance.ShellCmd{
							Run: "my command --foo=bar --baz",
						},
					},
				},
				// Condition: `command.stdout == "output"`,
			},
			module:            fmt.Sprintf(commandModule, `input.command.stdout == "output"`),
			commandExitCode:   0,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: commandutils.GetDefaultShell().Name,
			expectCommandArgs: append(commandutils.GetDefaultShell().Args, "my command --foo=bar --baz"),
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": json.Number("0"),
				},
			},
		},
		{
			name: "custom shell run",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Command: &compliance.Command{
						ShellCmd: &compliance.ShellCmd{
							Run: "my command --foo=bar --baz",
							Shell: &compliance.BinaryCmd{
								Name: "zsh",
								Args: []string{"-someoption", "-c"},
							},
						},
					},
				},
				// Condition: `command.stdout == "output"`,
			},
			module:            fmt.Sprintf(commandModule, `input.command.stdout == "output"`),
			commandExitCode:   0,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: "zsh",
			expectCommandArgs: []string{"-someoption", "-c", "my command --foo=bar --baz"},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": json.Number("0"),
				},
			},
		},
		{
			name: "execution failure",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Command: &compliance.Command{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "myCommand",
							Args: []string{"--foo=bar", "--baz"},
						},
					},
				},
				// Condition: `command.stdout == "output"`,
			},
			module:            fmt.Sprintf(commandModule, `input.command.stdout == "output"`),
			commandExitCode:   -1,
			commandError:      errors.New("some failure"),
			expectCommandName: "myCommand",
			expectCommandArgs: []string{"--foo=bar", "--baz"},
			expectReport: &compliance.Report{
				Passed: false,
				Error:  errors.New("Binary command 'myCommand' execution failed, error: myCommand"),
			},
		},
		{
			name: "non-zero return code",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Command: &compliance.Command{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "myCommand",
							Args: []string{"--foo=bar", "--baz"},
						},
					},
				},
				// Condition: `command.exitCode == 2`,
			},
			module:            fmt.Sprintf(commandModule, `input.command.exitCode == 2`),
			commandExitCode:   2,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: "myCommand",
			expectCommandArgs: []string{"--foo=bar", "--baz"},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": json.Number("2"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
