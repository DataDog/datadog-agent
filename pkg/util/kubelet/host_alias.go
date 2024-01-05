// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
)

// GetHostAliases uses the "kubelet" hostname provider to fetch the kubernetes alias
func GetHostAliases(ctx context.Context) ([]string, error) {
	name, err := GetHostname(ctx)
	if err != nil {
		return nil, fmt.Errorf("couldn't extract a host alias from the kubelet: %w", err)
	}
	if err := validate.ValidHostname(name); err != nil {
		return nil, fmt.Errorf("host alias from kubelet is not valid: %w", err)
	}
	return []string{name}, nil
}

// GetMetaClusterNameText returns the clusterName text for the agent status output. Returns "" if the feature kubernetes is not activated
func GetMetaClusterNameText(ctx context.Context, hostname string) string {
	compliantClusterName, initialClusterName := getRFC1123CompliantClusterName(ctx, hostname)
	if compliantClusterName != initialClusterName {
		return fmt.Sprintf("%s (original name: %s)", compliantClusterName, initialClusterName)
	}
	return compliantClusterName
}
