// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/stretchr/testify/require"
)

// CheckInstallation run tests to check the installation of the agent
func CheckInstallation(t *testing.T, client *TestClient) {
	t.Run("example config file", func(tt *testing.T) {
		exampleFilePath := client.Helper.GetConfigFolder() + fmt.Sprintf("%s.example", client.Helper.GetConfigFileName())

		_, err := client.FileManager.FileExists(exampleFilePath)
		require.NoError(tt, err, "Example config file should be present")
	})

	t.Run("datdog-agent binary", func(tt *testing.T) {
		binaryPath := client.Helper.GetBinaryPath()

		_, err := client.FileManager.FileExists(binaryPath)
		require.NoError(tt, err, "datadog-agent binary should be present")
	})
}

// CheckSigningKeys ensures datadog-signing-keys package is installed
func CheckSigningKeys(t *testing.T, client *TestClient) {
	t.Run("datadog-signing-keys package", func(tt *testing.T) {
		if _, err := client.Host.Execute("dpkg --version"); err != nil {
			tt.Skip()
		}
		_, err := client.Host.Execute(("dpkg -l datadog-signing-keys"))
		require.NoError(tt, err, "datadog-signing-keys package should be installed")
	})
}

// CheckInstallationMajorAgentVersion run tests to check the installation of an agent has the correct major version
func CheckInstallationMajorAgentVersion(t *testing.T, client *TestClient, expectedVersion string) bool {
	return t.Run("Check datadog-agent status version", func(tt *testing.T) {
		versionRegexPattern := regexp.MustCompile(`(?m:^(IoT )?Agent \(v([0-9]).*\)$)`)
		tmpCmd := fmt.Sprintf("sudo %s status", client.Helper.GetBinaryPath())
		output, err := client.ExecuteWithRetry(tmpCmd)
		require.NoError(tt, err, "datadog-agent status failed")
		matchList := versionRegexPattern.FindStringSubmatch(output)
		require.NotEmpty(tt, matchList, "wasn't able to retrieve datadog-agent major version on the following output : %s", output)
		require.True(tt, matchList[2] == expectedVersion, "Expected datadog-agent major version %s got %s", expectedVersion, matchList[2])
	})
}

// CheckAgentVersion run tests to check that the agent has the correct version
func (client *TestClient) CheckAgentVersion(t *testing.T, expected string) bool {
	return t.Run("Check datadog-agent version", func(t *testing.T) {
		versionRegexPattern := regexp.MustCompile("^(?m:IoT )?Agent (.*?) -")
		output := client.AgentClient.Version()
		matchList := versionRegexPattern.FindStringSubmatch(output)
		require.Len(t, matchList, 2, "wasn't able to retrieve datadog-agent version on the following output : %s", output)

		// regex to get major.minor.build parts
		expectedVersion, err := version.New(expected, "")
		require.NoErrorf(t, err, "invalid expected version %s", expected)
		actualVersion, err := version.New(matchList[1], "")
		require.NoErrorf(t, err, "invalid actual version %s", matchList[1])

		require.Equal(t, expectedVersion.GetNumberAndPre(), actualVersion.GetNumberAndPre(), "Expected datadog-agent version %s got %s", expectedVersion, actualVersion)
	})
}

// CheckUninstallation runs check to see if the agent uninstall properly
func CheckUninstallation(t *testing.T, client *TestClient) {

	t.Run("no running processes", func(tt *testing.T) {
		running, err := RunningAgentProcesses(client)
		require.NoError(tt, err)
		require.Empty(tt, running, "no agent process should be running")
	})

	t.Run("remove install directory", func(tt *testing.T) {
		installFolderPath := client.Helper.GetInstallFolder()

		entries, err := client.FileManager.ReadDir(installFolderPath)
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		require.Error(tt, err, "should not find anything in install folder, found %v dir entries.\nContent: %+v ", len(entries), strings.Join(names, ", "))
	})
}
