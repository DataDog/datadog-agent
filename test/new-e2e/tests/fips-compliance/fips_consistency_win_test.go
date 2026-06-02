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
	defer s.CleanupOnSetupFailure()

	agentHost := s.Env().WindowsVM
	dockerHost := s.Env().LinuxDockerVM

	// Set up fips-server on the Linux Docker VM
	composeFilePath := "/tmp/docker-compose.yaml"
	s.fipsServer = newFIPSServer(dockerHost, composeFilePath)

	_, err := dockerHost.WriteFile(composeFilePath, bytes.ReplaceAll(dockerFipsServerCompose, []byte("{APPS_VERSION}"), []byte(apps.Version)))
	require.NoError(s.T(), err)

	err = windowsCommon.EnableFIPSMode(agentHost)
	require.NoError(s.T(), err)

	agentPackage, err := windowsAgent.GetPackageFromEnv(windowsAgent.WithFlavor("fips"))
	require.NoError(s.T(), err)
	s.T().Logf("Using Agent: %#v", agentPackage)
	logFile := filepath.Join(s.SessionOutputDir(), "install.log")

	// Point dd_url at the fips-server so the cipher test can observe the daemon's
	// TLS negotiation without --local. WithInstallOnly defers service start so we
	// can append skip_ssl_validation before the daemon makes its first connection.
	fipsServerURL := fmt.Sprintf("https://%s:443", dockerHost.HostOutput.Address)
	_, err = windowsAgent.InstallAgent(agentHost,
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithInstallLogFile(logFile),
		// The connectivity-datadog-core-endpoints diagnoses require a non-empty API key
		windowsAgent.WithZeroAPIKey(),
		windowsAgent.WithDdURL(fipsServerURL),
		// Defer service start so we can write skip_ssl_validation before first connect
		windowsAgent.WithInstallOnly("1"),
	)
	require.NoError(s.T(), err)

	s.installPath, err = windowsAgent.GetInstallPathFromRegistry(agentHost)
	require.NoError(s.T(), err)

	// skip_ssl_validation is not a standard MSI parameter; append it directly.
	// The fips-server uses a self-signed certificate.
	_, err = agentHost.Execute(`Add-Content -Path 'C:\ProgramData\Datadog\datadog.yaml' -Value 'skip_ssl_validation: true'`)
	require.NoError(s.T(), err)

	// Start the service now; it initializes FIPS mode from the registry (currently enabled).
	err = windowsCommon.StartService(agentHost, "datadogagent")
	require.NoError(s.T(), err)

	// Start the fips-server so the image is pulled before any test runs
	s.fipsServer.Start(s.T(), cipherTestCase{cert: "rsa"})
}

// TestFIPSConsistency validates that:
//  1. The agent reports FIPS Mode: enabled when started with the registry key set
//  2. Disabling the registry key while the agent is running does not change the
//     reported FIPS mode (FIPS is determined at agent init and is sticky)
//  3. After the registry key is disabled, the running daemon continues to use only
//     FIPS-compliant TLS ciphers
func (s *fipsConsistencyWinSuite) TestFIPSConsistency() {
	agentHost := s.Env().WindowsVM

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

	// With the registry key disabled, verify the running daemon still uses only
	// FIPS-compliant ciphers. The daemon's dd_url was configured at install time to
	// point at the fips-server, so the diagnose command (without --local) makes the
	// running daemon establish the connection, proving its cipher selection is locked
	// in at initialization and is not affected by the registry key change.
	s.Run("FIPSCiphersAfterRegistryDisable", func() {
		for _, tc := range testcases {
			s.Run(fmt.Sprintf("FIPS enabled testing '%v -c %v' (should connect %v)", tc.cert, tc.cipher, tc.want), func() {
				s.fipsServer.Start(s.T(), tc)
				s.T().Cleanup(func() {
					s.fipsServer.Stop()
				})

				out, _ := s.execAgentCommand("agent.exe", "diagnose --include connectivity-datadog-core-endpoints")
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
