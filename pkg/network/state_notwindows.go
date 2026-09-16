// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

// dropStaleFlowFailures is a no-op on non-Windows platforms. The stale-failure
// condition it guards against is caused by a race in the ddnpm Windows kernel
// driver that leaves terminated flows in its openFlows table indefinitely;
// that driver bug does not exist on Linux or macOS.
// See state_windows.go for the full implementation and root-cause explanation.
func dropStaleFlowFailures(_ *ConnectionStats, _, _ StatCounters) {}
