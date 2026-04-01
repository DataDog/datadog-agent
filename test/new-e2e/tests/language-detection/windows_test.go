// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsLanguageDetectionSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestWindowsLanguageDetectionSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsLanguageDetectionSuite{},
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithRunOptions(
					ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
					ec2.WithAgentOptions(agentparams.WithAgentConfig(processConfigStr)),
				),
			),
		),
	)
}

func (s *windowsLanguageDetectionSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// Install chocolatey (same pattern as windows_test.go in process tests)
	stdout, err := s.Env().RemoteHost.Execute("Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iwr https://community.chocolatey.org/install.ps1 -UseBasicParsing | iex")
	require.NoErrorf(s.T(), err, "Failed to install chocolatey: %s", stdout)

	// Install Python
	stdout, err = s.Env().RemoteHost.Execute(`C:\ProgramData\chocolatey\bin\choco.exe install -y python3 --no-progress`)
	require.NoErrorf(s.T(), err, "Failed to install python: %s", stdout)
}

func (s *windowsLanguageDetectionSuite) TestPythonDetectionWindows() {
	// Resolve the full python path since SSH sessions don't inherit choco's PATH update
	pythonPath := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		`$env:Path = [System.Environment]::GetEnvironmentVariable('Path','Machine'); (Get-Command python).Source`,
	))
	s.T().Logf("Using python at: %s", pythonPath)

	// Start Python in a persistent SSH session so it stays alive (same pattern as runWindowsCommand in process tests)
	session, stdin, _, err := s.Env().RemoteHost.Start(pythonPath + ` -c "import time; time.sleep(300)"`)
	require.NoError(s.T(), err, "Failed to start python")
	s.T().Cleanup(func() {
		_ = session.Close()
		_ = stdin.Close()
	})

	// Verify that the agent detects python via the remote_process_collector source in workload-list
	assert.Eventually(s.T(),
		func() bool {
			wl, err := s.Env().RemoteHost.Execute(`& "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe" workload-list`)
			if err != nil {
				return false
			}
			return strings.Contains(wl, "Language: python") && strings.Contains(wl, "remote_process_collector")
		},
		2*time.Minute, 5*time.Second,
		"language detection did not detect python via remote_process_collector in workload-list",
	)
}
