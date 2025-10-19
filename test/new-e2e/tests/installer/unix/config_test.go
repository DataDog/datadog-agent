// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/fleetbackend"
)

type configSuite struct {
	packageBaseSuite
}

func testConfig(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &configSuite{
		packageBaseSuite: newPackageSuite("config", os, arch, method),
	}
}

func (s *configSuite) TestConfig() {
	s.agent.MustInstall(agent.WithRemoteUpdates())
	defer s.agent.MustUninstall()

	err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
		DeploymentID:   "123",
		FileOperations: []fleetbackend.FileOperation{{FileOperationType: fleetbackend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
	})
	require.NoError(s.T(), err)
	err = s.backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	config, err := s.agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
}
