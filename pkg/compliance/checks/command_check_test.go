// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows

package checks

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"
)

type commandFixture struct {
	name string

	resource compliance.Resource

	commandExitCode int
	commandOutput   string
	commandError    error

	expectCommandName string
	expectCommandArgs []string

	expectReport *compliance.Report
}

func (f *commandFixture) mockRunCommand(t *testing.T) commandRunnerFunc {
	return func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
		assert.Equal(t, f.expectCommandName, name)
		assert.Equal(t, f.expectCommandArgs, args)
		return f.commandExitCode, []byte(f.commandOutput), f.commandError
	}
}

func (f *commandFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	commandRunner = f.mockRunCommand(t)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	commandCheck, err := newResourceCheck(env, "rule-id", f.resource)
	assert.NoError(err)

	reports := commandCheck.check(env)
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
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					Command: &compliance.Command{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "myCommand",
							Args: []string{"--foo=bar", "--baz"},
						},
					},
				},
				Condition: `command.stdout == "output"`,
			},
			commandExitCode:   0,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: "myCommand",
			expectCommandArgs: []string{"--foo=bar", "--baz"},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": 0,
				},
			},
		},
		{
			name: "shell run",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					Command: &compliance.Command{
						ShellCmd: &compliance.ShellCmd{
							Run: "my command --foo=bar --baz",
						},
					},
				},
				Condition: `command.stdout == "output"`,
			},
			commandExitCode:   0,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: getDefaultShell().Name,
			expectCommandArgs: append(getDefaultShell().Args, "my command --foo=bar --baz"),
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": 0,
				},
			},
		},
		{
			name: "custom shell run",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
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
				Condition: `command.stdout == "output"`,
			},
			commandExitCode:   0,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: "zsh",
			expectCommandArgs: []string{"-someoption", "-c", "my command --foo=bar --baz"},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": 0,
				},
			},
		},
		{
			name: "execution failure",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					Command: &compliance.Command{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "myCommand",
							Args: []string{"--foo=bar", "--baz"},
						},
					},
				},
				Condition: `command.stdout == "output"`,
			},
			commandExitCode:   -1,
			commandError:      errors.New("some failure"),
			expectCommandName: "myCommand",
			expectCommandArgs: []string{"--foo=bar", "--baz"},
			expectReport: &compliance.Report{
				Passed: false,
				Error:  errors.New("command 'Binary command: myCommand, args: [--foo=bar --baz]' execution failed, error: some failure"),
			},
		},
		{
			name: "non-zero return code",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					Command: &compliance.Command{
						BinaryCmd: &compliance.BinaryCmd{
							Name: "myCommand",
							Args: []string{"--foo=bar", "--baz"},
						},
					},
				},
				Condition: `command.exitCode == 2`,
			},
			commandExitCode:   2,
			commandOutput:     "output",
			commandError:      nil,
			expectCommandName: "myCommand",
			expectCommandArgs: []string{"--foo=bar", "--baz"},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"command.exitCode": 2,
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
