// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

// GetMetadata returns the metadata for CRI runtime such as cri_name and cri_version.
func GetMetadata() (map[string]string, error) {
	cu, err := GetUtil()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"cri_name":    cu.GetRuntime(),
		"cri_version": cu.GetRuntimeVersion(),
	}, nil
}
