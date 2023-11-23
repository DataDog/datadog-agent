// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets && !windows

package secrets

import (
	"context"
	"os/exec"
)

// commandContext sets up an exec.Cmd for running with a context
func commandContext(ctx context.Context, name string, arg ...string) (*exec.Cmd, func(), error) {
	return exec.CommandContext(ctx, name, arg...), func() {}, nil
}
