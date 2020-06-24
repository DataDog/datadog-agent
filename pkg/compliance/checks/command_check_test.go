// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package checks

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/stretchr/testify/assert"
)

type commandFixture struct {
	name string

	command *compliance.Command

	commandExitCode int
	commandOutput   string
	commandError    error
	expCommandName  string
	expCommandArgs  []string
	expKV           compliance.KVMap
	expError        error
}

func (f *commandFixture) mockRunCommand(t *testing.T) commandRunnerFunc {
	return func(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
		assert.Equal(t, f.expCommandName, name)
		assert.ElementsMatch(t, f.expCommandArgs, args)
		return f.commandExitCode, []byte(f.commandOutput), f.commandError
	}
}

func (f *commandFixture) run(t *testing.T) {
	t.Helper()

	commandRunner = f.mockRunCommand(t)

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)
	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	if f.expKV != nil {
		env.On("Reporter").Return(reporter)
		reporter.On(
			"Report",
			newTestRuleEvent(
				[]string{"check_kind:command"},
				f.expKV,
			),
		).Once()
	}

	check, err := newCommandCheck(newTestBaseCheck(env, checkKindCommand), f.command)
	assert.NoError(t, err)

	err = check.Run()
	assert.Equal(t, f.expError, err)
}

func TestCommandCheck(t *testing.T) {
	tests := []commandFixture{
		{
			name: "Test binary run",
			command: &compliance.Command{
				BinaryCmd: &compliance.BinaryCmd{
					Name: "myCommand",
					Args: []string{"--foo=bar", "--baz"},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
			},
			commandExitCode: 0,
			commandOutput:   "output",
			commandError:    nil,
			expCommandName:  "myCommand",
			expCommandArgs:  []string{"--foo=bar", "--baz"},
			expKV: compliance.KVMap{
				"myCommandOutput": "output",
				"exitCode":        "0",
			},
		},
		{
			name: "Test shell run",
			command: &compliance.Command{
				ShellCmd: &compliance.ShellCmd{
					Run: "my command --foo=bar --baz",
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
			},
			commandExitCode: 0,
			commandOutput:   "output",
			commandError:    nil,
			expCommandName:  getDefaultShell().Name,
			expCommandArgs:  append(getDefaultShell().Args, "my command --foo=bar --baz"),
			expKV: compliance.KVMap{
				"myCommandOutput": "output",
				"exitCode":        "0",
			},
		},
		{
			name: "Test custom shell run",
			command: &compliance.Command{
				ShellCmd: &compliance.ShellCmd{
					Run: "my command --foo=bar --baz",
					Shell: &compliance.BinaryCmd{
						Name: "zsh",
						Args: []string{"-someoption", "-c"},
					},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
			},
			commandExitCode: 0,
			commandOutput:   "output",
			commandError:    nil,
			expCommandName:  "zsh",
			expCommandArgs:  []string{"-someoption", "-c", "my command --foo=bar --baz"},
			expKV: compliance.KVMap{
				"myCommandOutput": "output",
				"exitCode":        "0",
			},
		},
		{
			name: "Test execution failure",
			command: &compliance.Command{
				BinaryCmd: &compliance.BinaryCmd{
					Name: "myCommand",
					Args: []string{"--foo=bar", "--baz"},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
			},
			commandExitCode: -1,
			commandError:    fmt.Errorf("some failure"),
			expCommandName:  "myCommand",
			expCommandArgs:  []string{"--foo=bar", "--baz"},
			expKV:           nil,
			expError:        fmt.Errorf("check-1: command 'Binary command: myCommand, args: [--foo=bar --baz]' execution failed, error: some failure"),
		},
		{
			name: "Test non-zero return code (no filter)",
			command: &compliance.Command{
				BinaryCmd: &compliance.BinaryCmd{
					Name: "myCommand",
					Args: []string{"--foo=bar", "--baz"},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
			},
			commandExitCode: 2,
			commandOutput:   "output",
			commandError:    nil,
			expCommandName:  "myCommand",
			expCommandArgs:  []string{"--foo=bar", "--baz"},
			expKV: compliance.KVMap{
				"myCommandOutput": "output",
				"exitCode":        "2",
			},
		},
		{
			name: "Test allowed non-zero return code",
			command: &compliance.Command{
				BinaryCmd: &compliance.BinaryCmd{
					Name: "myCommand",
					Args: []string{"--foo=bar", "--baz"},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
				Filter: []compliance.CommandFilter{
					{
						Include: &compliance.CommandCondition{
							ExitCode: 2,
						},
					},
				},
			},
			commandExitCode: 2,
			commandOutput:   "output",
			commandError:    nil,
			expCommandName:  "myCommand",
			expCommandArgs:  []string{"--foo=bar", "--baz"},
			expKV: compliance.KVMap{
				"myCommandOutput": "output",
				"exitCode":        "2",
			},
		},
		{
			name: "Test not allowed non-zero return code",
			command: &compliance.Command{
				BinaryCmd: &compliance.BinaryCmd{
					Name: "myCommand",
					Args: []string{"--foo=bar", "--baz"},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
				Filter: []compliance.CommandFilter{
					{
						Include: &compliance.CommandCondition{
							ExitCode: 2,
						},
					},
				},
			},
			commandExitCode: 3,
			commandOutput:   "output",
			commandError:    fmt.Errorf("some failure"),
			expCommandName:  "myCommand",
			expCommandArgs:  []string{"--foo=bar", "--baz"},
			expKV:           nil,
			expError:        fmt.Errorf("check-1: command 'Binary command: myCommand, args: [--foo=bar --baz]' returned with exitcode: 3 (not reportable), error: some failure"),
		},
		{
			name: "Test output is too large",
			command: &compliance.Command{
				BinaryCmd: &compliance.BinaryCmd{
					Name: "myCommand",
					Args: []string{"--foo=bar", "--baz"},
				},
				Report: compliance.Report{
					{
						As: "myCommandOutput",
					},
				},
				MaxOutputSize: 50,
			},
			commandExitCode: 0,
			commandOutput:   "outputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutputoutput",
			expCommandName:  "myCommand",
			expCommandArgs:  []string{"--foo=bar", "--baz"},
			expKV:           nil,
			expError:        fmt.Errorf("check-1: command 'Binary command: myCommand, args: [--foo=bar --baz]' output is too large: 120, won't be reported"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
