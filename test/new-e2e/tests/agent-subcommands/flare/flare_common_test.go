// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"runtime"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	fakeflare "github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
)

type baseFlareSuite struct {
	e2e.BaseSuite[environments.Host]
}

// BeforeTest will be called before each test
func (v *baseFlareSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	// Reset the fakeintake between tests so GetLatestFlare() can't pick up a flare
	// left over from a previous test (or the framework's post-failure diagnose flare).
	require.NoError(v.T(), v.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (v *baseFlareSuite) TestFlareDefaultFiles() {
	flareArgs := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, flareArgs)

	assert.NotContains(v.T(), logs, "Error")

	AssertFilesExist(v.T(), flare, DefaultFlareFiles)
	AssertFilesExist(v.T(), flare, DefaultLogFiles)
	AssertFilesExist(v.T(), flare, DefaultConfigFiles)
	AssertFilesExist(v.T(), flare, DefaultMetadataFlareFiles)
	AssertFoldersExist(v.T(), flare, DefaultFlareFolders)

	AssertFilesExist(v.T(), flare, NonLocalFlareFiles)
	AssertFilesExist(v.T(), flare, NonLocalMetadataFlareFiles)

	AssertLogsFolderOnlyContainsLogFile(v.T(), flare)
	AssertEtcFolderOnlyContainsConfigFile(v.T(), flare)

	if runtime.GOOS != "linux" {
		AssertFilesExist(v.T(), flare, []string{"process-agent_tagger-list.json"})
		AssertFileContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
		AssertFileNotContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
		AssertFileNotContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")
	}
}

func (v *baseFlareSuite) TestLocalFlareDefaultFiles() {
	flareArgs := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--local"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, flareArgs)

	assert.Contains(v.T(), logs, "Initiating flare locally.")
	assert.NotContains(v.T(), logs, "Error")
	AssertFilesExist(v.T(), flare, []string{"local"})

	AssertFilesExist(v.T(), flare, DefaultFlareFiles)
	AssertFilesExist(v.T(), flare, DefaultLogFiles)
	AssertFilesExist(v.T(), flare, DefaultConfigFiles)
	AssertFilesExist(v.T(), flare, DefaultMetadataFlareFiles)
	AssertFoldersExist(v.T(), flare, DefaultFlareFolders)

	AssertLogsFolderOnlyContainsLogFile(v.T(), flare)
	AssertEtcFolderOnlyContainsConfigFile(v.T(), flare)
}

func (v *baseFlareSuite) TestFlareProfiling() {
	if runtime.GOOS != "windows" {
		// wake up the trace-agent
		resp, _ := v.Env().RemoteHost.NewHTTPClient().Get("http://localhost:8126/services")
		if resp != nil {
			resp.Body.Close()
		}
	}

	args := agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send", "--profile", "31",
		"--profile-blocking", "--profile-blocking-rate", "5000", "--profile-mutex", "--profile-mutex-fraction", "200"})
	flare, logs := requestAgentFlareAndFetchFromFakeIntake(v, args)

	assert.Contains(v.T(), logs, "Setting runtime_mutex_profile_fraction to 200")
	assert.Contains(v.T(), logs, "Setting runtime_block_profile_rate to 5000")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from core.")
	assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from security-agent.")
	AssertFilesExist(v.T(), flare, ProfilingFiles)

	if runtime.GOOS != "linux" {
		assert.Contains(v.T(), logs, "Getting a 31s profile snapshot from process.")
		AssertFilesExist(v.T(), flare, ProfilingNonLinuxFiles)
	}
}

func requestAgentFlareAndFetchFromFakeIntake(v *baseFlareSuite, flareArgs ...agentclient.AgentArgsOption) (fakeflare.Flare, string) {
	v.T().Helper()

	// Wait for the fakeintake to be ready to avoid 503 when sending the flare
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().FakeIntake.Client().GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")

	const maxAttempts = 3
	var flareLog string
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		flareLog, err = v.Env().Agent.Client.FlareWithError(flareArgs...)
		if err == nil {
			break
		}
		if attempt == maxAttempts {
			require.NoError(v.T(), err, "flare command failed after %d attempts", maxAttempts)
		}
		v.T().Logf("flare command failed (attempt %d/%d): %v; flushing fakeintake before retrying", attempt, maxAttempts, err)
		// Flush so a partial/failed attempt can't leave a stale payload for GetLatestFlare() to pick up later.
		require.NoError(v.T(), v.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	}

	flare, err := v.Env().FakeIntake.Client().GetLatestFlare()
	require.NoError(v.T(), err)

	return flare, flareLog
}
