// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cri

package cri

// GetMetadata returns the metadata for CRI runtime.
func GetMetadata() (map[string]string, error) {
	return nil, nil
}
