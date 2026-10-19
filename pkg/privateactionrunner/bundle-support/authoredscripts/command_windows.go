// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"context"
	"errors"
	"os/exec"
)

func platformCommand(_ context.Context, _ []string) (*exec.Cmd, error) {
	return nil, errors.New("authored-script commands are not supported on Windows")
}

func executionDeniedError(err error) error {
	return err
}

func configureCommand(_ *exec.Cmd) {}

func startCommand(_ *exec.Cmd) (commandController, error) {
	return nil, errors.New("authored-script execution is not supported on Windows")
}
