// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetRemoteConfigurationAllowedIntegrationsEmptyConf(t *testing.T) {
	// EMPTY configuration
	config := configmock.New(t)
	require.Equal(t, map[string]bool{}, getRemoteConfigurationAllowedIntegrations(config))
}

func TestGetRemoteConfigurationAllowedIntegrationsEnvVarsAllowList(t *testing.T) {
	t.Setenv("DD_REMOTE_CONFIGURATION_AGENT_INTEGRATIONS_ALLOW_LIST", "[\"POSTgres\", \"redisDB\"]")
	config := configmock.New(t)
	require.Equal(t,
		map[string]bool{"postgres": true, "redisdb": true},
		getRemoteConfigurationAllowedIntegrations(config),
	)
}

func TestGetRemoteConfigurationAllowedIntegrationsEnvVarBlockList(t *testing.T) {
	t.Setenv("DD_REMOTE_CONFIGURATION_AGENT_INTEGRATIONS_ALLOW_LIST", "[\"POSTgres\", \"redisDB\"]")
	t.Setenv("DD_REMOTE_CONFIGURATION_AGENT_INTEGRATIONS_BLOCK_LIST", "[\"mySQL\", \"redisDB\"]")
	config := configmock.New(t)
	require.Equal(t,
		map[string]bool{"postgres": true, "redisdb": false, "mysql": false},
		getRemoteConfigurationAllowedIntegrations(config),
	)
}
