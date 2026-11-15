// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"fmt"
	"os/exec"
)

func platformCommand(ctx context.Context, command []string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, command[0], command[1:]...), nil
}

func executionDeniedError(err error) error {
	return fmt.Errorf("authored-script execution was denied by the host; check file permissions and application-allowlisting policies such as fapolicyd: %w", err)
}
