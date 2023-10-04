// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"regexp"
	"testing"

	e2eClient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/stretchr/testify/require"
)

// CheckIntegrationInstall run test to test installation of integrations
func CheckIntegrationInstall(t *testing.T, client *ExtendedClient) {

	requirementIntegrationPath := "/opt/datadog-agent/requirements-agent-release.txt"

	ciliumRegex := regexp.MustCompile(`datadog-cilium==.*`)
	freezeContent := client.VMClient.Execute(fmt.Sprintf("cat %s", requirementIntegrationPath))
	freezeContent = ciliumRegex.ReplaceAllString(freezeContent, "datadog-cilium==2.2.1")
	client.VMClient.Execute(fmt.Sprintf(`sudo bash -c " echo '%s' > %s"`, freezeContent, requirementIntegrationPath))

	t.Run("uninstall installed package", func(tt *testing.T) {
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))

		freezeRequirement := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"remove", "-r", "datadog-cilium"}))
		freezeRequirementNew := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))

		require.Contains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before removal integration should be in freeze")
		require.NotContains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after removal integration should not be in freeze")
	})

	t.Run("install a new package", func(tt *testing.T) {
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"remove", "-r", "datadog-cilium"}))

		freezeRequirement := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))
		freezeRequirementNew := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))

		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before install integration should not be in freeze")
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after install integration should be in freeze")
	})

	t.Run("upgrade a package", func(tt *testing.T) {
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))

		freezeRequirement := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.3.0"}))
		freezeRequirementNew := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))

		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.3.0", "before update integration should not be in 2.3.0")
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.3.0", "after update integration should be in 2.3.0")
	})

	t.Run("downgrade a package", func(tt *testing.T) {
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.3.0"}))

		freezeRequirement := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))
		client.AgentClient.Integration(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.1"}))
		freezeRequirementNew := client.AgentClient.Integration(e2eClient.WithArgs([]string{"freeze"}))

		require.NotContains(tt, freezeRequirement, "datadog-cilium==2.2.1", "before downgrade integration should not be in 2.2.1")
		require.Contains(tt, freezeRequirementNew, "datadog-cilium==2.2.1", "after downgrade integration should be in 2.2.1")
	})

	t.Run("downgrade to older version than shipped", func(tt *testing.T) {
		_, err := client.AgentClient.IntegrationWithError(e2eClient.WithArgs([]string{"install", "-r", "datadog-cilium==2.2.0"}))
		require.Error(tt, err, "should raise error when trying to install version older than the one shipped")
	})
}
