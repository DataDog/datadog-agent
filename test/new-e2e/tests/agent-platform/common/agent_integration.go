// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/stretchr/testify/require"
)

// CheckIntegrationInstall run test to test installation of integrations
func CheckIntegrationInstall(t *testing.T, client *TestClient) {
	requirementIntegrationPath := client.Helper.GetInstallFolder() + "requirements-agent-release.txt"

	ciliumRegex := regexp.MustCompile(`datadog-cilium==.*`)
	freezeContent, err := client.FileManager.ReadFile(requirementIntegrationPath)
	require.NoError(t, err)

	freezeContent = ciliumRegex.ReplaceAll(freezeContent, []byte("datadog-cilium==2.2.1"))
	_, err = client.FileManager.WriteFile(requirementIntegrationPath, freezeContent)
	require.NoError(t, err)

	t.Run("install-uninstall package", func(tt *testing.T) {
		client.AgentClient.Integration(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))

		freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.Contains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before removal integration should be in freeze")

		client.AgentClient.Integration(agentclient.WithArgs([]string{"remove", "-r", "datadog-cilium"}))

		freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.NotContains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after removal integration should not be in freeze")
	})

	t.Run("upgrade a package", func(tt *testing.T) {
		client.AgentClient.Integration(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))

		freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.3.0", "before update integration should not be in 2.3.0")

		client.AgentClient.Integration(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.3.0"}))

		freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.3.0", "after update integration should be in 2.3.0")
	})

	t.Run("downgrade a package", func(tt *testing.T) {
		client.AgentClient.Integration(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.3.0"}))

		freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before downgrade integration should not be in 2.2.1")

		client.AgentClient.Integration(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))

		freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after downgrade integration should be in 2.2.1")
	})

	t.Run("downgrade to older version than shipped", func(tt *testing.T) {
		_, err := client.AgentClient.IntegrationWithError(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.0"}))
		require.Error(tt, err, "should raise error when trying to install version older than the one shipped")
	})
}
