// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package kubelet

// GetMetadata returns metadata about the kubelet runtime such as the kubelet_version.
func GetMetadata() (map[string]string, error) {
	return nil, nil
}
