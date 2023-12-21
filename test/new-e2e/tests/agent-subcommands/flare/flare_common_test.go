// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type baseFlareSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (v *baseFlareSuite) TestFlareDefaultFiles() {
	flare := requestAgentFlareAndFetchFromFakeIntake(v.T(), v.Env().Agent.Client, v.Env().FakeIntake.Client(), agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

	assertFilesExist(v.T(), flare, defaultFlareFiles)
	assertFilesExist(v.T(), flare, defaultLogFiles)
	assertFilesExist(v.T(), flare, defaultConfigFiles)
	assertFoldersExist(v.T(), flare, defaultFlareFolders)

	assertLogsFolderOnlyContainsLogFile(v.T(), flare)
	assertEtcFolderOnlyContainsConfigFile(v.T(), flare)

	assertFileContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
	assertFileNotContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
	assertFileNotContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")
}

func requestAgentFlareAndFetchFromFakeIntake(t *testing.T, agent agentclient.Agent, fakeintake *fi.Client, flareArgs ...agentclient.AgentArgsOption) flare.Flare {
	// Wait for the fakeintake to be ready to avoid 503 when sending the flare
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.NoError(c, fakeintake.GetServerHealth())
	}, 5*time.Minute, 20*time.Second, "timedout waiting for fakeintake to be healthy")

	_ = agent.Flare(flareArgs...)

	flare, err := fakeintake.GetLatestFlare()
	require.NoError(t, err)

	return flare
}
