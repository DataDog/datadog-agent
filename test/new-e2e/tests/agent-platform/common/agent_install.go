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

		exampleFilePath := client.Helper.GetConfigFolder() + "datadog.yaml.example"

		_, err := client.FileManager.FileExists(exampleFilePath)
		require.NoError(tt, err, "Example config file should be present")
	})

	t.Run("datdog-agent binary", func(tt *testing.T) {

		binaryPath := client.Helper.GetBinaryPath()

		_, err := client.FileManager.FileExists(binaryPath)
		require.NoError(tt, err, "datadog-agent binary should be present")
	})

	t.Run("datadog-signing-keys package", func(tt *testing.T) {
		if _, err := client.VMClient.ExecuteWithError("dpkg --version"); err != nil {
			tt.Skip()
		}
		_, err := client.VMClient.ExecuteWithError(("dpkg -l datadog-signing-keys"))
		require.NoError(tt, err, "datadog-signing-keys package should be installed")
	})
}

// CheckInstallationInstallScript run tests to check the installation of the agent with the install script
func CheckInstallationInstallScript(t *testing.T, client *TestClient) {

	t.Run("site config attribute", func(tt *testing.T) {
		configFilePath := client.Helper.GetConfigFolder() + "datadog.yaml"

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

	t.Run("remove the agent", func(tt *testing.T) {
		_, err := client.PkgManager.Remove("datadog-agent")
		require.NoError(tt, err, "should uninstall the agent")
	})

	t.Run("no agent process running", func(tt *testing.T) {
		agentProcesses := []string{"datadog-agent", "system-probe", "security-agent"}
		for _, process := range agentProcesses {
			_, err := client.VMClient.ExecuteWithError(fmt.Sprintf("pgrep -f %s", process))
			require.Error(tt, err, fmt.Sprintf("process %s should not be running", process))
		}
	})

	t.Run("remove install directory", func(tt *testing.T) {
		installFolderPath := client.Helper.GetInstallFolder()

		foundFiles, err := client.FileManager.FindFileInFolder(installFolderPath)
		require.Error(tt, err, "should not find anything in install folder, found: ", foundFiles)
	})

}
