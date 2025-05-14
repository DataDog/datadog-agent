// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	"context"
	"errors"
	"fmt"
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
	span, _ := StartSpanFromContext(ctx, fmt.Sprintf("exec.%s", name))
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
	err = c.Cmd.Run()
	exitErr := &exec.ExitError{}
	if !errors.As(err, &exitErr) {
		return err
	}
	c.span.SetTag("exit_code", exitErr.ExitCode())
	c.span.SetTag("stderr", string(exitErr.Stderr))
	return err
}
