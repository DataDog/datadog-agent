// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver && !linux

package common

import (
	"os"
)

// GetSelfPodName returns hostname from DD_POD_NAME in helm chart, if not found, use os.hostname
func GetSelfPodName() (string, error) {
	if podName, ok := os.LookupEnv("DD_POD_NAME"); ok {
		return podName, nil
	}

	return os.Hostname()
}
