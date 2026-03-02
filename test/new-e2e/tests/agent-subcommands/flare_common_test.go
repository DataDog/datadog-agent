// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"runtime"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	fakeflare "github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	flarehelpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-subcommands/flare"
)

type baseFlareSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (v *baseFlareSuite) TestFlareDefaultFiles() {
	flareArgs := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, flareArgs)

	assert.NotContains(v.T(), logs, "Error")

	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultFlareFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultLogFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultConfigFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultMetadataFlareFiles)
	flarehelpers.AssertFoldersExist(v.T(), flare, flarehelpers.DefaultFlareFolders)

	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.NonLocalFlareFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.NonLocalMetadataFlareFiles)

	flarehelpers.AssertLogsFolderOnlyContainsLogFile(v.T(), flare)
	flarehelpers.AssertEtcFolderOnlyContainsConfigFile(v.T(), flare)

	if runtime.GOOS != "linux" {
		flarehelpers.AssertFilesExist(v.T(), flare, []string{"process-agent_tagger-list.json"})
		flarehelpers.AssertFileContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
		flarehelpers.AssertFileNotContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
		flarehelpers.AssertFileNotContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")
	}
}

func (v *baseFlareSuite) TestLocalFlareDefaultFiles() {
	flareArgs := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--local"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, flareArgs)

	assert.Contains(v.T(), logs, "Initiating flare locally.")
	assert.NotContains(v.T(), logs, "Error")
	flarehelpers.AssertFilesExist(v.T(), flare, []string{"local"})

	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultFlareFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultLogFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultConfigFiles)
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.DefaultMetadataFlareFiles)
	flarehelpers.AssertFoldersExist(v.T(), flare, flarehelpers.DefaultFlareFolders)

	flarehelpers.AssertLogsFolderOnlyContainsLogFile(v.T(), flare)
	flarehelpers.AssertEtcFolderOnlyContainsConfigFile(v.T(), flare)
}

func (v *baseFlareSuite) TestFlareProfiling() {
	if runtime.GOOS != "windows" {
		// wake up the trace-agent
		v.Env().RemoteHost.NewHTTPClient().Get("http://localhost:8126/services")
	}

	args := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--profile", "31",
		"--profile-blocking", "--profile-blocking-rate", "5000", "--profile-mutex", "--profile-mutex-fraction", "200"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, args)

	assert.Contains(v.T(), logs, "Setting runtime_mutex_profile_fraction to 200")
	assert.Contains(v.T(), logs, "Setting runtime_block_profile_rate to 5000")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from core.")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from security-agent.")
	flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.ProfilingFiles)

	if runtime.GOOS != "linux" {
		assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from process.")
		flarehelpers.AssertFilesExist(v.T(), flare, flarehelpers.ProfilingNonLinuxFiles)
	}
}

func requestAgentFlareAndFetchFromFakeIntake(v *baseFlareSuite, flareArgs ...agentclient.AgentArgsOption) (fakeflare.Flare, string) {
	v.T().Helper()

	// Wait for the fakeintake to be ready to avoid 503 when sending the flare
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().FakeIntake.Client().GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")

	flareLog := v.Env().Agent.Client.Flare(flareArgs...)

	flare, err := v.Env().FakeIntake.Client().GetLatestFlare()
	require.NoError(v.T(), err)

	return flare, flareLog
}
