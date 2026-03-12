// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"
)

const agentHistoryDir = "/etc/datadog-agent/.config_history"

type configHistorySuite struct {
	suite.FleetSuite
}

func newConfigHistorySuite() e2e.Suite[environments.Host] {
	return &configHistorySuite{}
}

func TestFleetConfigHistory(t *testing.T) {
	suite.Run(t, newConfigHistorySuite, suite.LinuxPlatforms)
}

// SetupTest skips the test on Windows since config history is Linux-only.
func (s *configHistorySuite) SetupTest() {
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		s.T().Skip("Config history is Linux-only")
	}
}

// enableHistory appends config_history.enabled: true to the stable datadog.yaml.
func (s *configHistorySuite) enableHistory() {
	_, err := s.Env().RemoteHost.Execute(`printf '\nconfig_history:\n  enabled: true\n' | sudo tee -a /etc/datadog-agent/datadog.yaml > /dev/null`)
	require.NoError(s.T(), err)
}

// listDiffFiles returns the absolute paths of all .diff files in the history directory.
func (s *configHistorySuite) listDiffFiles() []string {
	out, err := s.Env().RemoteHost.Execute(`find ` + agentHistoryDir + ` -maxdepth 1 -name '*.diff' -type f 2>/dev/null`)
	require.NoError(s.T(), err)
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// TestConfigHistory_ExperimentCreatesEntry verifies that starting a config
// experiment writes a single unified-diff entry into the history directory,
// tagged with the deployment ID and showing the changed lines.
func (s *configHistorySuite) TestConfigHistory_ExperimentCreatesEntry() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.enableHistory()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "hist-exp-1",
		FileOperations: []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		},
	}, nil)
	require.NoError(s.T(), err)

	dirExists, err := s.Host.DirExists(agentHistoryDir)
	require.NoError(s.T(), err)
	require.True(s.T(), dirExists, "history directory must exist after WriteExperiment")

	diffFiles := s.listDiffFiles()
	require.Len(s.T(), diffFiles, 1, "expected one diff entry after WriteExperiment")

	raw, err := s.Env().RemoteHost.ReadFile(diffFiles[0])
	require.NoError(s.T(), err)
	body := string(raw)
	assert.Contains(s.T(), body, "# deployment_id: hist-exp-1")
	assert.Contains(s.T(), body, "+log_level: debug")
}

// TestConfigHistory_PromoteDoesNotAddEntry verifies that promoting an experiment
// to stable does not write an additional history entry.
func (s *configHistorySuite) TestConfigHistory_PromoteDoesNotAddEntry() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.enableHistory()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "hist-promote-1",
		FileOperations: []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		},
	}, nil)
	require.NoError(s.T(), err)

	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	assert.Len(s.T(), s.listDiffFiles(), 1, "promote must not append a diff entry")
}

// TestConfigHistory_RollbackPreservesEntry verifies that rolling back a config
// experiment (StopConfigExperiment) does not erase the diff entry that was
// written when the experiment started, and that the configuration is reverted.
func (s *configHistorySuite) TestConfigHistory_RollbackPreservesEntry() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.enableHistory()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "hist-rollback-1",
		FileOperations: []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		},
	}, nil)
	require.NoError(s.T(), err)

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
	require.Len(s.T(), s.listDiffFiles(), 1, "expected one diff entry before rollback")

	err = s.Backend.StopConfigExperiment()
	require.NoError(s.T(), err)

	// Config must be reverted to the pre-experiment value.
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "info", config["log_level"])

	// The original entry must still be present; rollback must not erase history.
	diffFiles := s.listDiffFiles()
	require.Len(s.T(), diffFiles, 1, "history entry must persist after rollback")

	raw, err := s.Env().RemoteHost.ReadFile(diffFiles[0])
	require.NoError(s.T(), err)
	body := string(raw)
	assert.Contains(s.T(), body, "# deployment_id: hist-rollback-1")
	assert.Contains(s.T(), body, "+log_level: debug")
}

// TestConfigHistory_MultipleExperimentsMultipleEntries verifies that each
// successive experiment-promote cycle produces a distinct history entry.
func (s *configHistorySuite) TestConfigHistory_MultipleExperimentsMultipleEntries() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.enableHistory()

	levels := []string{"debug", "warn", "error"}
	for i, level := range levels {
		err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
			DeploymentID: fmt.Sprintf("hist-multi-%d", i),
			FileOperations: []backend.FileOperation{
				{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: fmt.Appendf(nil, `{"log_level": "%s"}`, level)},
			},
		}, nil)
		require.NoError(s.T(), err)
		err = s.Backend.PromoteConfigExperiment()
		require.NoError(s.T(), err)
	}

	assert.Len(s.T(), s.listDiffFiles(), len(levels), "expected one diff entry per experiment")
}

// TestConfigHistory_MultipleRollbacks verifies that rolling back multiple
// successive experiments leaves a complete audit trail. Each experiment that
// was started — whether later promoted or rolled back — must have a diff entry.
func (s *configHistorySuite) TestConfigHistory_MultipleRollbacks() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.enableHistory()

	// First experiment: promote.
	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "hist-rb-promote",
		FileOperations: []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		},
	}, nil)
	require.NoError(s.T(), err)
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)
	require.Len(s.T(), s.listDiffFiles(), 1)

	// Second experiment: roll back.
	err = s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "hist-rb-rollback",
		FileOperations: []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "warn"}`)},
		},
	}, nil)
	require.NoError(s.T(), err)

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "warn", config["log_level"])

	err = s.Backend.StopConfigExperiment()
	require.NoError(s.T(), err)

	// Config reverts to the promoted value.
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])

	// Both experiments must be represented in history.
	diffFiles := s.listDiffFiles()
	require.Len(s.T(), diffFiles, 2, "both experiments must have a diff entry regardless of rollback")
}
