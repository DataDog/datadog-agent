// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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
func (client *TestClient) CheckAgentVersion(t *testing.T, expectedVersion string) bool {
	return t.Run("Check datadog-agent status version", func(t *testing.T) {
		versionRegexPattern := regexp.MustCompile("^(?m:IoT )?Agent (.*?) -")
		output := client.AgentClient.Version()
		matchList := versionRegexPattern.FindStringSubmatch(output)
		require.Len(t, matchList, 2, "wasn't able to retrieve datadog-agent version on the following output : %s", output)
		require.True(t, matchList[1] == expectedVersion, "Expected datadog-agent version %s got %s", expectedVersion, matchList[1])
	})
}

// CheckInstallationInstallScript run tests to check the installation of the agent with the install script
func CheckInstallationInstallScript(t *testing.T, client *TestClient) {
	t.Run("site config attribute", func(tt *testing.T) {
		configFilePath := client.Helper.GetConfigFolder() + client.Helper.GetConfigFileName()

		var configYAML map[string]any
		config, err := client.FileManager.ReadFile(configFilePath)
		require.NoError(tt, err)

		err = yaml.Unmarshal([]byte(config), &configYAML)
		require.NoError(tt, err)
		require.Equal(tt, configYAML["site"], "datadoghq.eu")
	})

	t.Run("install info file", func(tt *testing.T) {
		installInfoFilePath := client.Helper.GetConfigFolder() + "install_info"

		var installInfoYaml map[string]map[string]string
		installInfo, err := client.FileManager.ReadFile(installInfoFilePath)
		require.NoError(tt, err)

		err = yaml.Unmarshal([]byte(installInfo), &installInfoYaml)
		require.NoError(tt, err)
		toolVersionRegex := regexp.MustCompile(`^install_script_agent\d+$`)
		installerVersionRegex := regexp.MustCompile(`^install_script-\d+\.\d+\.\d+(.post)?$`)
		installMethodJSON := installInfoYaml["install_method"]

		require.True(tt, toolVersionRegex.MatchString(installMethodJSON["tool_version"]))
		require.True(tt, installerVersionRegex.MatchString(installMethodJSON["installer_version"]))
		require.Equal(tt, installMethodJSON["tool"], "install_script")
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
		require.Error(tt, err, "should not find anything in install folder, found %v dir entries ", len(entries))
	})
}
