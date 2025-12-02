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
	span *Span
}

// CommandContext runs a command using exec.CommandContext and adds telemetry
func CommandContext(ctx context.Context, name string, args ...string) *TracedCmd {
	span, _ := StartSpanFromContext(ctx, "exec."+name)
	span.SetTag("name", name)
	span.SetTag("args", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	return &TracedCmd{
		Cmd:  cmd,
		span: span,
	}
}

// Run runs the command and finishes the span
func (c *TracedCmd) Run() (err error) {
	defer func() { c.span.Finish(err) }()
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
			c.span.SetTag("exit_code", exitErr.ExitCode())
		}
		return fmt.Errorf("%s\n%s\n%w", stdout.String(), stderr.String(), err)
	}
	return nil
}
