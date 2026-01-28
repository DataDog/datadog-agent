// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// CheckIntegrationInstall run test to test installation of integrations
func CheckIntegrationInstall(t *testing.T, client *TestClient) {
	t.Run("integration", func(tt *testing.T) {
		flake.Mark(tt)
		requirementIntegrationPath := client.Helper.GetInstallFolder() + "requirements-agent-release.txt"

		ciliumRegex := regexp.MustCompile(`datadog-cilium==.*`)
		freezeContent, err := client.FileManager.ReadFile(requirementIntegrationPath)
		require.NoError(tt, err)

		freezeContent = ciliumRegex.ReplaceAll(freezeContent, []byte("datadog-cilium==4.0.0"))
		_, err = client.FileManager.WriteFile(requirementIntegrationPath, freezeContent)
		require.NoError(tt, err)

		tt.Run("install-uninstall package", func(ttt *testing.T) {
			installIntegration(ttt, client, "datadog-cilium==4.0.0")

			freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
			require.Contains(ttt, freezeRequirement, "datadog-cilium==4.0.0", "before removal integration should be in freeze")

			client.AgentClient.Integration(agentclient.WithArgs([]string{"remove", "-r", "datadog-cilium"}))

			freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
			require.NotContains(ttt, freezeRequirementNew, "datadog-cilium==4.0.0", "after removal integration should not be in freeze")
		})

		tt.Run("upgrade a package", func(ttt *testing.T) {
			installIntegration(ttt, client, "datadog-cilium==4.0.0")

			freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
			require.NotContains(ttt, freezeRequirement, "datadog-cilium==5.0.0", "before update integration should not be in 5.0.0")

			installIntegration(ttt, client, "datadog-cilium==5.0.0")

			freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
			require.Contains(ttt, freezeRequirementNew, "datadog-cilium==5.0.0", "after update integration should be in 5.0.0")
		})

		tt.Run("downgrade a package", func(ttt *testing.T) {
			installIntegration(ttt, client, "datadog-cilium==5.0.0")

			freezeRequirement := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
			require.NotContains(ttt, freezeRequirement, "datadog-cilium==4.0.0", "before downgrade integration should not be in 4.0.0")

			installIntegration(ttt, client, "datadog-cilium==4.0.0")

			freezeRequirementNew := client.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
			require.Contains(ttt, freezeRequirementNew, "datadog-cilium==4.0.0", "after downgrade integration should be in 4.0.0")
		})

		tt.Run("downgrade to older version than shipped", func(ttt *testing.T) {
			_, err := client.AgentClient.IntegrationWithError(agentclient.WithArgs([]string{"install", "-r", "datadog-cilium==3.6.0"}))
			require.Error(ttt, err, "should raise error when trying to install version older than the one shipped")
		})
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

	_, err := backoff.Retry(context.Background(), func() (any, error) {
		_, err := client.AgentClient.IntegrationWithError(agentclient.WithArgs([]string{"install", "--unsafe-disable-verification", "-r", integration}))
		return nil, err
	}, backoff.WithBackOff(backoff.NewConstantBackOff(interval)), backoff.WithMaxTries(uint(maxRetries)))

	require.NoError(t, err)
}
