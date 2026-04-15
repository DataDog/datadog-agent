// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package dnfv2

func computeBuiltinVariables(releaseVersion string) (map[string]string, error) {
	if releaseVersion == "" {
		releaseVersion = "2022.0.20220928"
	}

	return map[string]string{
		"arch":       "aarch64",
		"basearch":   "aarch64",
		"releasever": releaseVersion,
	}, nil
}
