// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package kubelet

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
)

// GetHostAlias uses the "kubelet" hostname provider to fetch the kubernetes alias
func GetHostAlias(ctx context.Context) (string, error) {
	name, err := HostnameProvider(ctx, nil)
	if err == nil && validate.ValidHostname(name) == nil {
		return name, nil
	}
	return "", fmt.Errorf("couldn't extract a host alias from the kubelet: %s", err)
}

// GetMetaClusterNameText returns the clusterName text for the agent status output. Returns "" if the feature kubernetes is not activated
func GetMetaClusterNameText(ctx context.Context, hostname string) string {
	compliantClusterName, initialClusterName := getRFC1123CompliantClusterName(ctx, hostname)
	if compliantClusterName != initialClusterName {
		return fmt.Sprintf("%s (original name: %s)", compliantClusterName, initialClusterName)
	}
	return compliantClusterName
}
