// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package coreagent provides a platform-specific helper to determine whether
// process checks run in the core agent.
package coreagent

// ProcessChecksRunInCoreAgent returns true on Linux where process checks
// (process, container, process discovery) always run in the core agent
// rather than the standalone process-agent.
func ProcessChecksRunInCoreAgent() bool {
	return true
}
