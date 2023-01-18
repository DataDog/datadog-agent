// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// RunCommands runs each command in cmds individually and returns the output
// as a []string, with each element corresponding to the respective command.
// If ignoreErrors is false, it will fail the test via t.Fatal immediately upon error.
// Otherwise, the output on errors will be logged via t.Log.
func RunCommands(tb testing.TB, cmds []string, ignoreErrors bool) []string {
	tb.Helper()
	var output []string

	for _, c := range cmds {
		out, err := RunCommand(c)
		output = append(output, out)
		if err != nil {
			if !ignoreErrors {
				tb.Fatal(err)
				return nil
			}
			tb.Log(err)
		}
	}
	return output
}

// RunCommand runs a single command
func RunCommand(cmd string) (string, error) {
	args := strings.Split(cmd, " ")
	c := exec.Command(args[0], args[1:]...)
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s returned %s: %s", c, err, out)
	}
	return string(out), nil
}

func StartCommand(cmd string) (*exec.Cmd, io.WriteCloser, error) {
	args := strings.Split(cmd, " ")
	c := exec.Command(args[0], args[1:]...)
	clientInput, err := c.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("could not get command %s standard input: %w", c, err)
	}

	err = c.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("could not start command %s: %w", c, err)
	}
	return c, clientInput, nil
}
