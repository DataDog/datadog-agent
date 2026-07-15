// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// TracedCmd is a wrapper around exec.Cmd that adds telemetry
type TracedCmd struct {
	*exec.Cmd
	span          *Span
	expectedCodes map[int]struct{}
}

// CommandContext runs a command using exec.CommandContext and adds telemetry
func CommandContext(ctx context.Context, name string, args ...string) *TracedCmd {
	span, _ := StartSpanFromContext(ctx, "exec."+name)
	span.SetTag("name", name)
	span.SetTag("args", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	return &TracedCmd{
		Cmd:           cmd,
		span:          span,
		expectedCodes: map[int]struct{}{0: {}},
	}
}

// WithExpectedExitCodes marks exit codes that should NOT flag the span as an
// error. The exit_code tag is still recorded and the error is still returned to
// the caller; only the span's error flag is suppressed for these codes.
// Exit code 0 (success) is always treated as expected.
func (c *TracedCmd) WithExpectedExitCodes(codes ...int) *TracedCmd {
	for _, code := range codes {
		c.expectedCodes[code] = struct{}{}
	}
	return c
}

// Run runs the command and finishes the span
func (c *TracedCmd) Run() (err error) {
	var expectedExit bool
	defer func() {
		if expectedExit {
			c.span.Finish(nil)
			return
		}
		c.span.Finish(err)
	}()
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	if c.Cmd.Stderr != nil {
		c.Cmd.Stderr = io.MultiWriter(c.Cmd.Stderr, &stderr)
	} else {
		c.Cmd.Stderr = &stderr
	}
	if c.Cmd.Stdout != nil {
		c.Cmd.Stdout = io.MultiWriter(c.Cmd.Stdout, &stdout)
	} else {
		c.Cmd.Stdout = &stdout
	}
	err = c.Cmd.Run()
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			code := exitErr.ExitCode()
			c.span.SetTag("exit_code", code)
			if _, ok := c.expectedCodes[code]; ok {
				expectedExit = true
				c.span.SetTag("expected_exit_code", true)
			}
		}
		return fmt.Errorf("%s\n%s\n%w", stdout.String(), stderr.String(), err)
	}
	return nil
}
