// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

// maybeSuppressWindowsLingeringFlow is a no-op on non-Windows platforms.
// The lingering-flow bug is specific to the ddnpm Windows kernel driver.
// See state_windows.go for the full implementation and root-cause explanation.
func maybeSuppressWindowsLingeringFlow(_ *ConnectionStats, _, _ StatCounters) {}
