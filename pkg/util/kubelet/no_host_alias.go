// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package kubelet

import (
	"context"
)

// GetHostAliases uses the "kubelet" hostname provider to fetch the kubernetes alias
func GetHostAliases(_ context.Context) ([]string, error) {
	panic("not called")
}

// GetMetaClusterNameText returns the clusterName text for the agent status output
func GetMetaClusterNameText(_ context.Context, _ string) string {
	panic("not called")
}
