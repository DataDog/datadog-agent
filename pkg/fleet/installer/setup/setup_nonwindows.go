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

// requestedAgentVersion is a no-op on non-Windows. The Linux and macOS
// install scripts resolve DD_AGENT_MAJOR/MINOR_VERSION upstream of the
// installer, so the in-process setup is always the right path here.
func requestedAgentVersion(_ *env.Env) (string, error) { return "", nil }

// runAgentInstaller is unreachable on non-Windows (callers gate on
// requestedAgentVersion != ""). Provided so setup.go compiles cross-platform.
func runAgentInstaller(_ context.Context, _ *env.Env, _, _ string) error {
	return nil
}

// applyAgentDistOptions is a no-op on non-Windows; distribution channel and
// pipeline options are handled upstream by the Linux/macOS install scripts.
func applyAgentDistOptions(_ *env.Env) error { return nil }
