// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package dcv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// CommandRunner directly executes the fixed DCV executable without a shell.
type CommandRunner struct{}

// Run executes one of the arguments selected by Collector.
func (CommandRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if err := validateCommandArgs(args); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, DefaultExecutable, args...)
	var output bytes.Buffer
	boundedOutput := &limitedWriter{writer: &output, remaining: maxOutputBytes + 1}
	command.Stdout = boundedOutput
	command.Stderr = boundedOutput
	if err := command.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("dcv command failed: %w: %s", err, truncate(output.String(), 1024))
	}
	if output.Len() > maxOutputBytes {
		return nil, errors.New("dcv command output exceeded 1 MiB")
	}
	return output.Bytes(), nil
}

type limitedWriter struct {
	mu        sync.Mutex
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	requested := len(p)
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	if len(p) > 0 {
		_, _ = w.writer.Write(p)
		w.remaining -= len(p)
	}
	return requested, nil
}
