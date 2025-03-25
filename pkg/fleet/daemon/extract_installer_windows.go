// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
)

// extractInstallerFromOCI downloads the installer binary from the agent package in the registry and returns an installer executor.
// No-op on windows
func extractInstallerFromOCI(_ context.Context, _ *env.Env, _, _ string) (*exec.InstallerExec, error) {
	return nil, fmt.Errorf("not supported on windows")
}
