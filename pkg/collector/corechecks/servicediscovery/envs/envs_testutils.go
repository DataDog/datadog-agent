// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package envs

import "fmt"

// NewVariables returns a new [Variables]
// for unit tests to verify that the input map has only target variables.
func NewVariables(vars map[string]string) Variables {
	for env := range vars {
		if _, ok := targets[env]; !ok {
			return Variables{}
		}
	}
	return Variables{
		vars: vars,
	}
}

// GetExpectedEnvs - return list of expected env. variables for testing.
func GetExpectedEnvs() []string {
	expectedEnvs := make([]string, 0, len(targets))

	for env := range targets {
		expectedEnvs = append(expectedEnvs, fmt.Sprintf("%s=true", env))
	}
	return expectedEnvs
}

// GetExpectedMap - return map of expected env. variables for testing.
func GetExpectedMap() map[string]string {
	expectedMap := make(map[string]string, len(targets))

	for env := range targets {
		expectedMap[env] = "true"
	}
	return expectedMap
}
