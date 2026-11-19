// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package clusteragent

// ResetGlobalClusterAgentClientForTest drops the process-wide Cluster Agent client, so a test in
// another package can start from a fresh client, as at Agent startup.
func ResetGlobalClusterAgentClientForTest() {
	resetGlobalClusterAgentClient()
}
