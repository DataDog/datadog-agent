// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package setup

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// handoffToRequestedAgentInstallerVersion is a no-op on non-Windows platforms. The Linux and
// macOS install scripts resolve DD_AGENT_MAJOR_VERSION/DD_AGENT_MINOR_VERSION
// upstream of the installer.
func handoffToRequestedAgentInstallerVersion(_ context.Context, _ *env.Env, _ string) (bool, error) {
	return false, nil
}
