// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
)

// parScriptConfigMarker is a script name that never appears in the packaged example config,
// so its presence proves the customer-edited file survived.
const parScriptConfigMarker = "e2e-fleet-upgrade-preserved"

// TestPARScriptConfigPreservedOnUpgrade verifies that a customer-edited Private Action Runner
// script config survives Fleet experiment rollback and promotion.
//
// Regression test for incident 61851: from 7.77.0 until 7.84.3 the packages owned the live
// script config, so every upgrade replaced customer edits with the packaged example. Packages
// now ship a `.example` template and only create the live file when it is missing.
//
// A single install covers both rollback and promotion to keep the Windows shard cheap.
func (s *upgradeSuite) TestPARScriptConfigPreservedOnUpgrade() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()

	configPath, examplePath := s.parScriptConfigPaths()

	// Fresh install initializes the live config from the template.
	s.Require().NotEmpty(s.readPARFile(configPath), "fresh install should create the live PAR script config")
	s.Require().NotEmpty(s.readPARFile(examplePath), "package should ship the PAR script config template")

	customConfig := s.customPARScriptConfig()
	s.writePARFile(configPath, customConfig)
	s.assertPARScriptConfig(configPath, customConfig, "before experiment")

	originalVersion, err := s.Agent.Version()
	s.Require().NoError(err)
	targetVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")

	// Rollback: start an experiment, then stop it.
	s.Require().NoError(s.Backend.StartExperiment("datadog-agent", targetVersion))
	s.assertPARScriptConfig(configPath, customConfig, "during experiment")
	s.Require().NoError(s.Backend.StopExperiment("datadog-agent"))
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 300*time.Second, 30*time.Second)
	s.assertPARScriptConfig(configPath, customConfig, "after experiment rollback")

	// Promotion: start the experiment again and promote it.
	s.Require().NoError(s.Backend.StartExperiment("datadog-agent", targetVersion))
	s.assertPARScriptConfig(configPath, customConfig, "during second experiment")
	s.Require().NoError(s.Backend.PromoteExperiment("datadog-agent"))
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, targetVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)
	s.assertPARScriptConfig(configPath, customConfig, "after promotion")

	s.Require().NotEmpty(s.readPARFile(examplePath), "upgraded package should still ship the PAR script config template")
}

// parScriptConfigPaths returns the live and template PAR script config paths for the host OS.
func (s *upgradeSuite) parScriptConfigPaths() (configPath string, examplePath string) {
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		configPath = "/etc/datadog-agent/private-action-runner/script-config.yaml"
	case e2eos.WindowsFamily:
		configPath = `C:\ProgramData\Datadog\private-action-runner\powershell-script-config.yaml`
	default:
		s.T().Fatalf("unsupported OS family: %v", s.Env().RemoteHost.OSFamily)
	}
	return configPath, configPath + ".example"
}

func (s *upgradeSuite) customPARScriptConfig() string {
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		return fmt.Sprintf(`schemaId: script-credentials-v1
runPredefinedPowershellScript:
  %s:
    script: |
      Write-Output "preserved"
`, parScriptConfigMarker)
	}
	return fmt.Sprintf(`schemaId: script-credentials-v1
runPredefinedScript:
  %s:
    command:
      - echo
      - preserved
`, parScriptConfigMarker)
}

func (s *upgradeSuite) readPARFile(path string) string {
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		content, err := s.Env().RemoteHost.ReadFile(path)
		s.Require().NoError(err, "failed to read %s", path)
		return string(content)
	}
	content, err := s.Env().RemoteHost.Execute("sudo cat " + path)
	s.Require().NoError(err, "failed to read %s", path)
	return content
}

func (s *upgradeSuite) writePARFile(path, content string) {
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		_, err := s.Env().RemoteHost.WriteFile(path, []byte(content))
		s.Require().NoError(err, "failed to write %s", path)
		return
	}
	// tee keeps the existing file's owner and mode.
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf("echo %s | base64 -d | sudo tee %s > /dev/null", encoded, path))
	s.Require().NoError(err, "failed to write %s", path)
}

func (s *upgradeSuite) assertPARScriptConfig(path, expected, stage string) {
	actual := s.readPARFile(path)
	s.Require().Equal(strings.TrimSpace(expected), strings.TrimSpace(actual),
		"PAR script config %s was modified %s", path, stage)
}
