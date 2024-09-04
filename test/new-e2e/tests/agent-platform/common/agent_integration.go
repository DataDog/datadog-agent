// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/cenkalti/backoff"
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
		installIntegration(tt, client, "datadog-cilium==2.2.1")

		freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.Contains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before removal integration should be in freeze")

		client.AgentClient.Integration(agentclient.WithArgs([]string{"remove", "-r", "datadog-cilium"}))

		freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.NotContains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after removal integration should not be in freeze")
	})

	t.Run("upgrade a package", func(tt *testing.T) {
		installIntegration(tt, client, "datadog-cilium==2.2.1")

		freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.3.0", "before update integration should not be in 2.3.0")

		installIntegration(tt, client, "datadog-cilium==2.3.0")

		freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.3.0", "after update integration should be in 2.3.0")
	})

	t.Run("downgrade a package", func(tt *testing.T) {
		installIntegration(tt, client, "datadog-cilium==2.3.0")

		freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before downgrade integration should not be in 2.2.1")

		installIntegration(tt, client, "datadog-cilium==2.2.1")

		freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after downgrade integration should be in 2.2.1")
	})

	t.Run("downgrade to older version than shipped", func(tt *testing.T) {
		_, err := client.AgentClient.IntegrationWithError(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.0"}))
		require.Error(tt, err, "should raise error when trying to install version older than the one shipped")
	})
}

func installIntegration(t *testing.T, client *TestClient, integration string) {
	// This operation can fail if the release pipeline in integrations-core is running.
	// The wheels are not generated in an atomic way and installation can fail during this process.
	// This is a first step, we could also install the integration from a local wheel to avoid the problem altogether but that's not what customers do.
	// Wheels can be unavailable for a few minutes.
	// Their release pipeline runs at least once a day at 5AM CET and can run during working hours if they need to release something.

	interval := 30 * time.Second
	maxRetries := 6

	err := backoff.Retry(func() error {
		_, err := client.AgentClient.IntegrationWithError(agentclient.WithArgs([]string{"install", "-r", integration}))
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(maxRetries)))

	require.NoError(t, err)
}
