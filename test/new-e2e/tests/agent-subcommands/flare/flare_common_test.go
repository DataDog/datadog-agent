// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type baseFlareSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (v *baseFlareSuite) TestFlareDefaultFiles() {
	flareArgs := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, flareArgs)

	assert.NotContains(v.T(), logs, "Error")

	assertFilesExist(v.T(), flare, defaultFlareFiles)
	assertFilesExist(v.T(), flare, defaultLogFiles)
	assertFilesExist(v.T(), flare, defaultConfigFiles)
	assertFilesExist(v.T(), flare, defaultMetadataFlareFiles)
	assertFoldersExist(v.T(), flare, defaultFlareFolders)

	assertFilesExist(v.T(), flare, nonLocalFlareFiles)
	assertFilesExist(v.T(), flare, nonLocalMetadataFlareFiles)

	assertLogsFolderOnlyContainsLogFile(v.T(), flare)
	assertEtcFolderOnlyContainsConfigFile(v.T(), flare)

	assertFileContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
	assertFileNotContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
	assertFileNotContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")
}

func (v *baseFlareSuite) TestLocalFlareDefaultFiles() {
	flareArgs := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--local"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, flareArgs)

	assert.Contains(v.T(), logs, "Initiating flare locally.")
	assert.NotContains(v.T(), logs, "Error")
	assertFilesExist(v.T(), flare, []string{"local"})

	assertFilesExist(v.T(), flare, defaultFlareFiles)
	assertFilesExist(v.T(), flare, defaultLogFiles)
	assertFilesExist(v.T(), flare, defaultConfigFiles)
	assertFilesExist(v.T(), flare, defaultMetadataFlareFiles)
	assertFoldersExist(v.T(), flare, defaultFlareFolders)

	assertLogsFolderOnlyContainsLogFile(v.T(), flare)
	assertEtcFolderOnlyContainsConfigFile(v.T(), flare)
}

func (v *baseFlareSuite) TestFlareProfiling() {
	args := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--profile", "31",
		"--profile-blocking", "--profile-blocking-rate", "5000", "--profile-mutex", "--profile-mutex-fraction", "200"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, args)

	assert.Contains(v.T(), logs, "Setting runtime_mutex_profile_fraction to 200")
	assert.Contains(v.T(), logs, "Setting runtime_block_profile_rate to 5000")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from core.")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from security-agent.")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from process.")

	assertFilesExist(v.T(), flare, profilingFiles)
}

func requestAgentFlareAndFetchFromFakeIntake(v *baseFlareSuite, flareArgs ...agentclient.AgentArgsOption) (flare.Flare, string) {
	// Wait for the fakeintake to be ready to avoid 503 when sending the flare
	assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().FakeIntake.Client().GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")

	flareLog := v.Env().Agent.Client.Flare(flareArgs...)

	flare, err := v.Env().FakeIntake.Client().GetLatestFlare()
	require.NoError(v.T(), err)

	return flare, flareLog
}
