// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"sync"
)

var (
	defaultEnvVarsIncludeList = []string{
		"DD_ENV",
		"DD_VERSION",
		"DD_SERVICE",
		"CHRONOS_JOB_NAME",
		"CHRONOS_JOB_OWNER",
		"NOMAD_TASK_NAME",
		"NOMAD_JOB_NAME",
		"NOMAD_GROUP_NAME",
		"NOMAD_NAMESPACE",
		"NOMAD_DC",
		"MESOS_TASK_ID",
		"ECS_CONTAINER_METADATA_URI",
		"ECS_CONTAINER_METADATA_URI_V4",
		"DOCKER_DD_AGENT", // included to be able to detect agent containers
		// Included to ease unit tests without requiring a mock
		"TEST_ENV",
	}

	envFilterOnce       sync.Once
	envFilterFromConfig EnvFilter
)

// EnvVarFilterFromConfig returns an EnvFilter based on the options present in the config
func EnvVarFilterFromConfig() EnvFilter {
	panic("not called")
}

// EnvFilter defines a filter for environment variables
type EnvFilter struct {
	includeVars map[string]struct{}
}

func newEnvFilter(includeVars []string) EnvFilter {
	panic("not called")
}

// IsIncluded returns whether the given env variable name is included
func (f EnvFilter) IsIncluded(envVarName string) bool {
	panic("not called")
}
