// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !kubelet

package hostinfo

import "fmt"

// GetHostAlias uses the "kubelet" hostname provider to fetch the kubernetes alias
func GetHostAlias() (string, error) {
	return "", fmt.Errorf("Kubernetes support not build: couldn't extract a host alias from the kubelet")
}
