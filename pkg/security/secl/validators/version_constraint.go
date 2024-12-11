// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validators holds validators related files
package validators

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ValidateAgentVersionConstraint validates an agent version constraint
func ValidateAgentVersionConstraint(constraint string) (*semver.Constraints, error) {
	trimmedConstraint := strings.TrimSpace(constraint)
	if trimmedConstraint == "" {
		return semver.NewConstraint("*")
	}
	return semver.NewConstraint(trimmedConstraint)
}
