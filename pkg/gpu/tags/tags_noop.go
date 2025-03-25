// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package tags provides GPU-related host tags
package tags

// GetTags returns empty list for non linux environments,
// that's because the go-nvml supports only linux.
func GetTags() []string {
	return nil
}
