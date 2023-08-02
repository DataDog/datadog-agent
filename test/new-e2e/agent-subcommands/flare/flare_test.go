// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type commandFlareSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestFlareSuite(t *testing.T) {
	e2e.Run(t, &commandFlareSuite{}, e2e.AgentStackDef(nil), params.WithDevMode())
}

func waitForAgentAndGetFlare(v *commandFlareSuite, flareArgs ...client.AgentArgsOption) flare.Flare {
	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	_ = v.Env().Agent.Flare(flareArgs...)

	flare, err := v.Env().Fakeintake.Client.GetLatestFlare()
	assert.NoError(v.T(), err)

	return flare
}

func (v *commandFlareSuite) TestFlareDefaultFiles() {
	flare := waitForAgentAndGetFlare(v, client.WithArgs("--email e2e@test.com --send"))

	assertFilesExist(v.T(), flare, defaultFlareFiles)
	assertFilesExist(v.T(), flare, defaultLogFiles)
	assertFilesExist(v.T(), flare, defaultConfigFiles)
	assertFoldersExist(v.T(), flare, defaultFlareFolders)

	assertProcessCheckShouldBeEnabled(v.T(), flare, "process", "process_config.process_collection.enabled", false)
	assertProcessCheckShouldBeEnabled(v.T(), flare, "container", "process_config.container_collection.enabled", true)
	assertProcessCheckShouldBeEnabled(v.T(), flare, "process_discovery", "process_config.process_discovery.enabled", true)

	assertLogsFolderOnlyContainsLogFile(v.T(), flare)
	assertEtcFolderOnlyContainsConfigFile(v.T(), flare)
}

//go:embed fixtures/all-configuration-scenario/etc/datadog-agent/datadog-agent.yaml
var agentConfiguration []byte

//go:embed fixtures/all-configuration-scenario/etc/datadog-agent/system-probe.yaml
var systemProbeConfiguration []byte

//go:embed fixtures/all-configuration-scenario/etc/datadog-agent/security-agent.yaml
var securityAgentConfiguration []byte

func (v *commandFlareSuite) TestFlareWithAllConfiguration() {

	var scenarioExpectedFiles = []string{
		"telemetry.log", // if telemetry.enabled
		"registry.json", // if Logs Agent is running
	}

	// XXX: use FileManager here instead
	// XXX: this test is expected to fail because 'etc/security-agent.yaml' is not found. See #18463
	v.Env().VM.Execute(fmt.Sprintf(`echo "%s" | sudo tee /etc/datadog-agent/system-probe.yaml`, systemProbeConfiguration))
	v.Env().VM.Execute(fmt.Sprintf(`echo "%s" | sudo tee /etc/datadog-agent/security-agent.yaml`, securityAgentConfiguration))
	v.UpdateEnv(e2e.AgentStackDef(nil, agentparams.WithAgentConfig(string(agentConfiguration))))

	err := v.Env().Agent.WaitForReady()
	assert.NoError(v.T(), err)

	_ = v.Env().Agent.Flare(client.WithArgs("--email e2e@test.com --send"))

	flare, err := v.Env().Fakeintake.Client.GetLatestFlare()
	assert.NoError(v.T(), err)

	assertFilesExist(v.T(), flare, scenarioExpectedFiles)
	assertFilesExist(v.T(), flare, allLogFiles)
	assertFilesExist(v.T(), flare, allConfigFiles)

	assertProcessCheckShouldBeEnabled(v.T(), flare, "process", "process_config.process_collection.enabled", true)
	assertProcessCheckShouldBeEnabled(v.T(), flare, "container", "process_config.container_collection.enabled", false)
	assertProcessCheckShouldBeEnabled(v.T(), flare, "process_discovery", "process_config.process_discovery.enabled", false)
}
