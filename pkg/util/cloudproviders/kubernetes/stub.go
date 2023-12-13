// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !kubelet

package kubernetes

import (
	"context"
)

var (
	// CloudProviderName contains the inventory name for Kubernetes (through the API server)
	CloudProviderName = "kubernetes"
)

// GetHostAliases returns the host aliases from the Kubernetes node annotations
func GetHostAliases(ctx context.Context) ([]string, error) {
	return nil, nil
}
