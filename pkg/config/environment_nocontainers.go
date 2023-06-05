// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(linux || windows)

package config

// IsAnyContainerFeaturePresent checks if any of known container features is present
func IsAnyContainerFeaturePresent() bool {
	return false
}

func detectContainerFeatures(features FeatureMap) {
}
