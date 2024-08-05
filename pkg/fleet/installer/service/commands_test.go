// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCommand(t *testing.T) {
	cmds := []*mockCmd{
		{
			runFunc: func() error {
				return &exec.ExitError{}
			},
			stderrData: "command execution failed",
		},
		{
			runFunc: func() error {
				return nil
			},
		},
		{
			runFunc: func() error {
				return &exec.Error{
					Err: fmt.Errorf("command not found"),
				}
			},
		},
	}

	for _, cmd := range cmds {
		fmt.Printf("%+v\n", cmd)
		err := runCommand(cmd)
		if cmd.runFunc() == nil {
			assert.NoError(t, err)
			continue
		}

		expected := cmd.stderrData
		if expected == "" {
			expected = cmd.runFunc().Error()
		}
		assert.Equal(t, fmt.Sprintf("command failed: %s", expected), err.Error())
	}
}
