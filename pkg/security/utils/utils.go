// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/Masterminds/semver/v3"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetAgentSemverVersion returns the agent version as a semver version
func GetAgentSemverVersion() (*semver.Version, error) {
	av, err := version.Agent()
	if err != nil {
		return nil, err
	}

	return semver.NewVersion(av.GetNumberAndPre())
}

// BoolTouint64 converts a boolean value to an uint64
func BoolTouint64(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}
