// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/Masterminds/semver"
)

// CheckAgentVersionConstraint checks that the semver constraint is satisfied by the agent version
func CheckAgentVersionConstraint(constraint string) (bool, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return true, nil
	}

	av, err := version.Agent()
	if err != nil {
		return false, err
	}

	agentVersion, err := semver.NewVersion(av.GetNumberAndPre())
	if err != nil {
		return false, err
	}

	semverConstraint, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, err
	}

	return semverConstraint.Check(agentVersion), nil
}
