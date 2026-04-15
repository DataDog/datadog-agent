// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/rshell/interp"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultProcPath         = "/proc"
	containerizedPathPrefix = "/host"
)

// statFn is the function used to check path existence. It defaults to os.Stat
// and can be overridden in tests.
var statFn = os.Stat

// RunCommandHandler implements the runCommand action.
type RunCommandHandler struct {
	allowedPaths []string
}

// NewRunCommandHandler creates a new RunCommandHandler.
func NewRunCommandHandler(allowedPaths []string) *RunCommandHandler {
	return &RunCommandHandler{
		allowedPaths: allowedPaths,
	}
}

// RunCommandInputs defines the inputs for the runCommand action.
type RunCommandInputs struct {
	Command         string   `json:"command"`
	AllowedCommands []string `json:"allowedCommands"`
}

// RunCommandOutputs defines the outputs for the runCommand action.
type RunCommandOutputs struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// streamingWriter is an io.Writer that publishes each complete line as an intermediate result
// while also writing everything to an underlying buffer for the final output.
type streamingWriter struct {
	buffer    *bytes.Buffer
	publisher types.IntermediateResultPublisher
	ctx       context.Context
	seqNum    int64
	pending   []byte
}

func (w *streamingWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	w.pending = append(w.pending, p...)
	for {
		idx := bytes.IndexByte(w.pending, '\n')
		if idx < 0 {
			break
		}
		line := string(w.pending[:idx])
		w.pending = w.pending[idx+1:]
		if pubErr := w.publisher.Publish(w.ctx, line, w.seqNum); pubErr != nil {
			log.Warnf("failed to publish intermediate result: %v", pubErr)
		}
		w.seqNum++
	}
	return n, err
}

// flush publishes any remaining partial line that didn't end with \n.
func (w *streamingWriter) flush() {
	if len(w.pending) > 0 {
		line := string(w.pending)
		w.pending = nil
		if pubErr := w.publisher.Publish(w.ctx, line, w.seqNum); pubErr != nil {
			log.Warnf("failed to publish final intermediate result: %v", pubErr)
		}
		w.seqNum++
	}
}

// Run executes the command through the rshell restricted interpreter.
// The environment is intentionally empty; no host environment variables are forwarded.
func (h *RunCommandHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RunCommandInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.Command == "" {
		return nil, errors.New("command is required")
	}

	log.Debugf("rshell runCommand: command=%q allowedCommands=%v allowedPaths=%v",
		inputs.Command, inputs.AllowedCommands, h.allowedPaths)

	prog, err := syntax.NewParser().Parse(strings.NewReader(inputs.Command), "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse command: %w", err)
	}

	for _, p := range h.allowedPaths {
		if _, err := statFn(p); err != nil {
			log.Warnf("path %q not found, rshell may fail to execute commands", p)
		}
	}

	var stdout, stderr bytes.Buffer
	var stdoutWriter io.Writer = &stdout

	publisher, hasPublisher := types.PublisherFromContext(ctx)
	var sw *streamingWriter
	if hasPublisher {
		sw = &streamingWriter{buffer: &stdout, publisher: publisher, ctx: ctx}
		stdoutWriter = sw
	}

	runner, err := interp.New(
		interp.StdIO(nil, stdoutWriter, &stderr),
		interp.AllowedPaths(h.allowedPaths),
		interp.ProcPath(resolveProcPath()),
		interp.AllowedCommands(append(inputs.AllowedCommands, "rshell:sleep", "rshell:ls")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}
	defer runner.Close()

	runErr := runner.Run(ctx, prog)

	if sw != nil {
		sw.flush()
	}

	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			return nil, runErr
		}
	}

	return &RunCommandOutputs{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// resolveProcPath returns the proc filesystem path appropriate for the current
// environment. In containerized deployments with host mounts, it returns
// /host/proc; otherwise it falls back to /proc.
func resolveProcPath() string {
	if env.IsContainerized() {
		hostProc := path.Join(containerizedPathPrefix, defaultProcPath)
		if _, err := statFn(hostProc); err == nil {
			return hostProc
		}
	}
	return defaultProcPath
}
