// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every AWS environment must resolve a Docker Hub mirror: an empty value would turn
// "datadog/agent-dev:main" into the unqualified path "/datadog/agent-dev:main".
func TestInternalDockerhubMirrorIsSetForEveryEnvironment(t *testing.T) {
	for _, envName := range []string{sandboxEnv, agentSandboxEnv, agentQAEnv, tsePlaygroundEnv} {
		t.Run(envName, func(t *testing.T) {
			ddInfra := getEnvironmentDefault(envName).ddInfra
			mirror := ddInfra.defaultInternalDockerhubMirror
			require.NotEmpty(t, mirror)

			if ddInfra.defaultInternalRegistry != "" {
				assert.Equal(t, ddInfra.defaultInternalRegistry+"/dockerhub", mirror)
			}
		})
	}
}

// Every AWS environment must resolve a registry for publicly released Datadog images:
// an empty value would turn "agent:latest" into the unqualified path "/agent:latest".
func TestDatadogPublicRegistryIsSetForEveryEnvironment(t *testing.T) {
	for _, envName := range []string{sandboxEnv, agentSandboxEnv, agentQAEnv, tsePlaygroundEnv} {
		t.Run(envName, func(t *testing.T) {
			ddInfra := getEnvironmentDefault(envName).ddInfra
			registry := ddInfra.defaultDatadogPublicRegistry
			require.NotEmpty(t, registry)

			// An environment with a pull-through cache must serve the images from it,
			// otherwise the pull leaves the account.
			if ddInfra.defaultInternalRegistry != "" {
				assert.Equal(t, ddInfra.defaultInternalRegistry+"/ecr-public/datadog", registry)
			}
		})
	}
}
