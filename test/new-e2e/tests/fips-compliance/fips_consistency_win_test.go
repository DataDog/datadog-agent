// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fipsConsistencyWinSuite struct {
	e2e.BaseSuite[multiVMEnv]

	installPath string
	fipsServer  fipsServer
}

// TestFIPSConsistencyWindowsSuite tests that the FIPS Agent maintains FIPS mode
// for its entire lifetime, even if the system FIPS registry key is disabled
// after the agent has started.
func TestFIPSConsistencyWindowsSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil)}
	e2e.Run(t, &fipsConsistencyWinSuite{}, suiteParams...)
}

func (s *fipsConsistencyWinSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	agentHost := s.Env().WindowsVM
	dockerHost := s.Env().LinuxDockerVM

	// Set up fips-server on the Linux Docker VM
	composeFilePath := "/tmp/docker-compose.yaml"
	s.fipsServer = newFIPSServer(dockerHost, composeFilePath)

	// Write docker-compose.yaml to disk
	_, err := dockerHost.WriteFile(composeFilePath, bytes.ReplaceAll(dockerFipsServerCompose, []byte("{APPS_VERSION}"), []byte(apps.Version)))
	require.NoError(s.T(), err)

	// Enable FIPS mode via registry before the agent starts
	err = windowsCommon.EnableFIPSMode(agentHost)
	require.NoError(s.T(), err)

	// Install FIPS Agent
	agentPackage, err := windowsAgent.GetPackageFromEnv(windowsAgent.WithFlavor("fips"))
	require.NoError(s.T(), err)
	s.T().Logf("Using Agent: %#v", agentPackage)
	logFile := filepath.Join(s.SessionOutputDir(), "install.log")
	_, err = windowsAgent.InstallAgent(agentHost,
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithInstallLogFile(logFile),
		// The connectivity-datadog-core-endpoints diagnoses require a non-empty API key
		windowsAgent.WithZeroAPIKey())
	require.NoError(s.T(), err)

	s.installPath, err = windowsAgent.GetInstallPathFromRegistry(agentHost)
	require.NoError(s.T(), err)

	// Start the fips-server so the image is pulled before any test runs
	s.fipsServer.Start(s.T(), cipherTestCase{cert: "rsa"})
}

// TestFIPSConsistency validates that:
//  1. The agent reports FIPS Mode: enabled when started with the registry key set
//  2. Disabling the registry key while the agent is running does not change the
//     reported FIPS mode (FIPS is determined at agent init and is sticky)
//  3. After the registry key is disabled, the agent continues to use only
//     FIPS-compliant TLS ciphers
func (s *fipsConsistencyWinSuite) TestFIPSConsistency() {
	agentHost := s.Env().WindowsVM
	dockerHost := s.Env().LinuxDockerVM

	s.Run("FIPSStatusEnabledAtStart", func() {
		status, err := s.execAgentCommand("agent.exe", "status")
		require.NoError(s.T(), err)
		assert.Contains(s.T(), status, "FIPS Mode: enabled")
	})

	// Disable the registry key while the agent is still running.
	err := windowsCommon.DisableFIPSMode(agentHost)
	require.NoError(s.T(), err)

	// FIPS mode is evaluated at agent init time, so disabling the registry key
	// mid-run should have no effect until the agent is restarted.
	s.Run("FIPSStatusStillEnabledAfterRegistryDisable", func() {
		status, err := s.execAgentCommand("agent.exe", "status")
		require.NoError(s.T(), err)
		assert.Contains(s.T(), status, "FIPS Mode: enabled",
			"FIPS mode should remain enabled for the lifetime of the agent process, "+
				"even after the registry key is disabled without a restart")
	})

	// With the registry key disabled, verify the agent still uses only FIPS-compliant ciphers.
	s.Run("FIPSCiphersAfterRegistryDisable", func() {
		// datadog/apps-fips-server creates a self-signed cert; point the diagnose
		// command at the container running on the Linux Docker VM.
		ddURL := fmt.Sprintf(`https://%s:443`, dockerHost.HostOutput.Address)
		agentEnv := client.EnvVar{
			"DD_SKIP_SSL_VALIDATION": "true",
			"DD_DD_URL":              ddURL,
		}

		for _, tc := range testcases {
			s.Run(fmt.Sprintf("FIPS enabled testing '%v -c %v' (should connect %v)", tc.cert, tc.cipher, tc.want), func() {
				s.fipsServer.Start(s.T(), tc)
				s.T().Cleanup(func() {
					s.fipsServer.Stop()
				})

				out, _ := s.execAgentCommand("agent.exe", "diagnose --include connectivity-datadog-core-endpoints --local", client.WithEnvVariables(agentEnv))
				require.NotContains(s.T(), out, "Total:0", "Expected diagnoses to run, ensure an API key is configured")

				serverLogs := s.fipsServer.Logs()
				if tc.want {
					assert.Contains(s.T(), serverLogs, "Negotiated cipher suite: "+tc.cipher)
				} else {
					assert.Contains(s.T(), serverLogs, "no cipher suite supported by both client and server")
				}
			})
		}
	})
}

func (s *fipsConsistencyWinSuite) execAgentCommand(executable, command string, options ...client.ExecuteOption) (string, error) {
	host := s.Env().WindowsVM
	s.Require().NotEmpty(s.installPath)

	agentPath := filepath.Join(s.installPath, "bin", executable)
	cmd := fmt.Sprintf(`& "%s" %s`, agentPath, command)
	return host.Execute(cmd, options...)
}
