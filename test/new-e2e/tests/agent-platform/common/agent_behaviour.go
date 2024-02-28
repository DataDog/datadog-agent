// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains basic test operation for agent-platform tests
package common

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/stretchr/testify/require"
)

// CheckAgentBehaviour runs test to check the agent is behaving as expected
func CheckAgentBehaviour(t *testing.T, client *TestClient) {
	t.Run("datadog-agent service running", func(tt *testing.T) {
		_, err := client.SvcManager.Status("datadog-agent")
		require.NoError(tt, err, "datadog-agent service should be running")
	})

	t.Run("datadog-agent checks running", func(tt *testing.T) {
		var statusOutputJSON map[string]any
		result := false
		for try := 0; try < 5 && !result; try++ {
			err := json.Unmarshal([]byte(client.AgentClient.Status(agentclient.WithArgs([]string{"-j"})).Content), &statusOutputJSON)
			require.NoError(tt, err)
			if runnerStats, ok := statusOutputJSON["runnerStats"]; ok {
				runnerStatsMap := runnerStats.(map[string]any)
				if checks, ok := runnerStatsMap["Checks"]; ok {
					checksMap := checks.(map[string]any)
					result = len(checksMap) > 0
				}
			}
			time.Sleep(1 * time.Second)
		}
		require.True(tt, result, "status output should contain running checks")
	})

	t.Run("status command infos", func(tt *testing.T) {
		statusOutput := client.AgentClient.Status().Content
		require.Contains(tt, statusOutput, "Forwarder")
		require.Contains(tt, statusOutput, "Host Info")
		require.Contains(tt, statusOutput, "DogStatsD")
	})

	t.Run("status command no errors", func(tt *testing.T) {
		statusOutput := client.AgentClient.Status().Content

		// API Key is invalid we should not check for the following error
		statusOutput = strings.ReplaceAll(statusOutput, "[ERROR] API Key is invalid", "API Key is invalid")
		require.NotContains(tt, statusOutput, "ERROR")
	})
}

// CheckDogstatdAgentBehaviour runs tests to check the agent behave properly with dogstatsd
func CheckDogstatdAgentBehaviour(t *testing.T, client *TestClient) {
	t.Run("dogstatsd service running", func(tt *testing.T) {
		_, err := client.SvcManager.Status("datadog-dogstatsd")
		require.NoError(tt, err, "dogstatsd service should be running")
	})

	t.Run("dogstatsd config file exists", func(tt *testing.T) {
		_, err := client.FileManager.FileExists(fmt.Sprintf("%s/%s", client.Helper.GetConfigFolder(), "dogstatsd.yaml"))
		require.NoError(tt, err, "dogstatsd config file should be present")
	})
}

// CheckAgentStops runs tests to check the agent can stop properly
func CheckAgentStops(t *testing.T, client *TestClient) {
	t.Run("stops", func(tt *testing.T) {
		_, err := client.SvcManager.Stop("datadog-agent")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-agent")
		require.Error(tt, err, "datadog-agent service should be stopped")
	})

	t.Run("refuse connections", func(tt *testing.T) {
		_, err := client.AgentClient.StatusWithError()
		require.Error(tt, err, "status should error")
	})

	t.Run("no running processes", func(tt *testing.T) {
		agentProcesses := []string{"datadog-agent", "system-probe", "security-agent"}
		for _, process := range agentProcesses {
			_, err := client.Host.Execute(fmt.Sprintf("pgrep -f %s", process))
			require.Error(tt, err, fmt.Sprintf("process %s should not be running", process))
		}
	})

	t.Run("starts after stop", func(tt *testing.T) {
		_, err := client.SvcManager.Start("datadog-agent")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-agent")
		require.NoError(tt, err, "datadog-agent should be running")
	})
}

// CheckDogstatsdAgentStops runs tests to check the agent can stop properly
func CheckDogstatsdAgentStops(t *testing.T, client *TestClient) {
	t.Run("stops", func(tt *testing.T) {
		_, err := client.SvcManager.Stop("datadog-dogstatsd")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-dogstatsd")
		require.Error(tt, err, "datadog-dogstatsd service should be stopped")
	})

	t.Run("no running processes", func(tt *testing.T) {
		dogstatsdProcesses := []string{"datadog-dogstatsd"}
		for _, process := range dogstatsdProcesses {
			_, err := client.Host.Execute(fmt.Sprintf("pgrep -f %s", process))
			require.Error(tt, err, fmt.Sprintf("process %s should not be running", process))
		}
	})

	t.Run("starts after stop", func(tt *testing.T) {
		_, err := client.SvcManager.Start("datadog-dogstatsd")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-dogstatsd")
		require.NoError(tt, err, "datadog-dogstatsd should be running")
	})
}

// CheckAgentRestarts runs tests to check the agent can restart properly
func CheckAgentRestarts(t *testing.T, client *TestClient) {
	t.Run("start when stopped", func(tt *testing.T) {
		// If the agent is not stopped yet, stop it
		if _, err := client.SvcManager.Status("datadog-agent"); err == nil {
			_, err := client.SvcManager.Stop("datadog-agent")
			require.NoError(tt, err)
		}

		_, err := client.SvcManager.Start("datadog-agent")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-agent")
		require.NoError(tt, err, "datadog-agent should restart when stopped")
	})

	t.Run("restart when running", func(tt *testing.T) {
		// If the agent is not started yet, start it
		if _, err := client.SvcManager.Status("datadog-agent"); err != nil {
			_, err := client.SvcManager.Start("datadog-agent")
			require.NoError(tt, err)
		}

		_, err := client.SvcManager.Restart("datadog-agent")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-agent")
		require.NoError(tt, err, "datadog-agent should restart when running")
	})
}

// CheckDogstatsdAgentRestarts runs tests to check the agent can restart properly
func CheckDogstatsdAgentRestarts(t *testing.T, client *TestClient) {
	t.Run("restart when stopped", func(tt *testing.T) {
		// If the agent is not stopped yet, stop it
		if _, err := client.SvcManager.Status("datadog-dogstatsd"); err == nil {
			_, err := client.SvcManager.Stop("datadog-dogstatsd")
			require.NoError(tt, err)
		}

		_, err := client.SvcManager.Start("datadog-dogstatsd")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-dogstatsd")
		require.NoError(tt, err, "datadog-dogstatsd should restart when stopped")
	})

	t.Run("restart when running", func(tt *testing.T) {
		// If the agent is not started yet, start it
		if _, err := client.SvcManager.Status("datadog-dogstatsd"); err != nil {
			_, err := client.SvcManager.Start("datadog-dogstatsd")
			require.NoError(tt, err)
		}

		_, err := client.SvcManager.Restart("datadog-dogstatsd")
		require.NoError(tt, err)

		_, err = client.SvcManager.Status("datadog-dogstatsd")
		require.NoError(tt, err, "datadog-dogstatsd should restart when running")
	})
}

// CheckAgentPython runs tests to check the agent use the correct python version
func CheckAgentPython(t *testing.T, client *TestClient, version string) {
	t.Run(fmt.Sprintf("set python version %s and restarts", version), func(tt *testing.T) {
		err := client.SetConfig("/etc/datadog-agent/datadog.yaml", "python_version", version)
		require.NoError(tt, err, "failed to set python version: ", err)

		_, err = client.SvcManager.Restart("datadog-agent")
		require.NoError(tt, err, "agent should be able to restart after editing python version")
	})

	t.Run(fmt.Sprintf("check python %s is used", version), func(tt *testing.T) {
		statusVersion, err := client.GetPythonVersion()
		require.NoError(tt, err)
		majorPythonVersion := strings.Split(statusVersion, ".")[0]

		require.Equal(tt, version, majorPythonVersion)
	})
}

// CheckApmEnabled runs tests to check the agent behave properly with APM enabled
func CheckApmEnabled(t *testing.T, client *TestClient) {
	t.Run("port bound apm enabled", func(tt *testing.T) {
		err := client.CheckPortBound(8126)
		require.NoError(tt, err, "port 8196 should be bound when APM is enabled")
	})
}

// CheckApmDisabled runs tests to check the agent behave properly when APM is disabled
func CheckApmDisabled(t *testing.T, client *TestClient) {
	t.Run("port not bound when disabled", func(tt *testing.T) {
		configFilePath := client.Helper.GetConfigFolder() + "datadog.yaml"

		err := client.SetConfig(configFilePath, "apm_config.enabled", "false")
		require.NoError(tt, err)

		_, err = client.SvcManager.Restart("datadog-agent")
		require.NoError(tt, err)

		err = client.CheckPortBound(8126)
		require.Error(tt, err, "port should not be bound when apm is manually disabled")
	})
}

// CheckCWSBehaviour runs tests to check the agent behave correctly when CWS is enabled
func CheckCWSBehaviour(t *testing.T, client *TestClient) {
	t.Run("enable CWS and restarts", func(tt *testing.T) {
		err := client.SetConfig(client.Helper.GetConfigFolder()+"system-probe.yaml", "runtime_security_config.enabled", "true")
		require.NoError(tt, err)
		err = client.SetConfig(client.Helper.GetConfigFolder()+"security-agent.yaml", "runtime_security_config.enabled", "true")
		require.NoError(tt, err)

		_, err = client.SvcManager.Restart("datadog-agent")
		require.NoError(tt, err, "datadog-agent should restart after CWS is enabled")
	})

	t.Run("security-agent is running", func(tt *testing.T) {
		var err error
		require.Eventually(tt, func() bool {
			_, err = client.Host.Execute("pgrep -f security-agent")
			return err == nil
		}, 1*time.Minute, 500*time.Millisecond, "security-agent should be running ", err)
	})

	t.Run("system-probe is running", func(tt *testing.T) {
		var err error
		require.Eventually(tt, func() bool {
			_, err = client.Host.Execute("pgrep -f system-probe")
			return err == nil
		}, 1*time.Minute, 500*time.Millisecond, "system-probe should be running ", err)
	})

	t.Run("system-probe and security-agent communicate", func(tt *testing.T) {
		var statusOutputJSON map[string]any
		var result bool
		for try := 0; try < 10 && !result; try++ {
			status, err := client.Host.Execute("sudo /opt/datadog-agent/embedded/bin/security-agent status -j")
			if err == nil {
				statusLines := strings.Split(status, "\n")
				status = strings.Join(statusLines[1:], "\n")
				err := json.Unmarshal([]byte(status), &statusOutputJSON)
				require.NoError(tt, err)
				if runtimeStatus, ok := statusOutputJSON["runtimeSecurityStatus"]; ok {
					if connected, ok := runtimeStatus.(map[string]any)["connected"]; ok {
						result = connected == true
					}
				}
			}

			time.Sleep(2 * time.Second)
		}
		require.True(tt, result, "system-probe and security-agent should communicate")
	})
}
