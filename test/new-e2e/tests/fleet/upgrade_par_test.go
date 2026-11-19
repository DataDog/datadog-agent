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
// script config survives Fleet experiment rollback and promotion. It guards against
// reintroducing the package-owned live config behind incident 61851, where upgrades replaced
// customer edits with the packaged example. Upgrades from affected versions (7.77.0 to 7.84.2)
// still lose the config by design and are not covered.
//
// Linux starts from this pipeline's DEB/RPM, so the package under test lays down the live file
// and the OCI experiment hooks must leave it alone. Windows starts from the released stable, so
// rolling back removes the experiment MSI: if that MSI owned the live file, removing it deletes
// the edit.
//
// A single install covers both rollback and promotion to keep the Windows shard cheap.
func (s *upgradeSuite) TestPARScriptConfigPreservedOnUpgrade() {
	isLinux := s.Env().RemoteHost.OSFamily == e2eos.LinuxFamily
	if isLinux {
		s.Agent.MustInstall(agent.WithRemoteUpdates())
	} else {
		s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	}
	defer s.Agent.MustUninstall()

	configPath, examplePath := s.parScriptConfigPaths()

	// Fresh install initializes the live config from the template.
	s.Require().NotEmpty(s.readPARFile(configPath), "fresh install should create the live PAR script config")
	s.Require().NotEmpty(s.readPARFile(examplePath), "package should ship the PAR script config template")

	customConfig := s.customPARScriptConfig()
	s.writePARFile(configPath, customConfig)
	s.assertPARScriptConfig(configPath, customConfig, "before experiment")
	permsBefore := s.parFilePermissions(configPath)

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
		status, err := s.Backend.RemoteConfigStatusPackage("datadog-agent")
		require.NoError(c, err)
		require.Empty(c, status.ExperimentVersion, "experiment should be fully stopped")
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
	s.Require().Equal(permsBefore, s.parFilePermissions(configPath), "PAR script config ownership or mode changed")

	if !isLinux {
		// On Linux the OCI experiment does not lay down /etc files, so this only means something
		// for the Windows MSI.
		s.Require().NotEmpty(s.readPARFile(examplePath), "upgraded package should still ship the PAR script config template")
	}
}

// parScriptConfigPaths returns the live and template PAR script config paths for the host OS.
func (s *upgradeSuite) parScriptConfigPaths() (configPath string, examplePath string) {
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		configPath = "/etc/datadog-agent/private-action-runner/script-config.yaml"
	case e2eos.WindowsFamily:
		// Fleet tests install with the default config root, see agent.installWindowsInstallExe.
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

// parFilePermissions returns owner, group and mode on Linux, and an empty string on Windows,
// where POSIX permissions do not apply.
func (s *upgradeSuite) parFilePermissions(path string) string {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		return ""
	}
	// sudo: the parent config directory is not traversable by the test user.
	perms, err := s.Env().RemoteHost.Execute("sudo stat -c '%U:%G %a' " + path)
	s.Require().NoError(err, "failed to stat %s", path)
	return strings.TrimSpace(perms)
}

func (s *upgradeSuite) assertPARScriptConfig(path, expected, stage string) {
	actual := s.readPARFile(path)
	s.Require().Equal(strings.TrimSpace(expected), strings.TrimSpace(actual),
		"PAR script config %s was modified %s", path, stage)
}
