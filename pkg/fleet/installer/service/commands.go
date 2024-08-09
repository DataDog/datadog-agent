// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type commandRunner interface {
	runWithError() error
}

type realCmd struct {
	*exec.Cmd
}

func (r *realCmd) runWithError() error {
	var errBuf bytes.Buffer
	r.Stderr = &errBuf
	err := r.Cmd.Run()
	if err == nil {
		return nil
	}

	if len(errBuf.Bytes()) == 0 {
		return fmt.Errorf("command failed: %s", err.Error())
	}

	return fmt.Errorf("command failed: %s \n%s", strings.TrimSpace(errBuf.String()), err.Error())
}

func newCommandRunner(ctx context.Context, name string, args ...string) commandRunner {
	cmd := exec.CommandContext(ctx, name, args...)
	return &realCmd{Cmd: cmd}
}
