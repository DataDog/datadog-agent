// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type commandFlareSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestFlareSuite(t *testing.T) {
	e2e.Run(t, &commandFlareSuite{}, e2e.FakeIntakeStackDef(nil), params.WithDevMode())
}

func waitForAgentAndGetFlare(v *commandFlareSuite, flareArgs ...client.AgentArgsOption) flare.Flare {
	_ = v.Env().Agent.Flare(flareArgs...)

	flare, err := v.Env().Fakeintake.Client.GetLatestFlare()
	assert.NoError(v.T(), err)

	return flare
}

func (v *commandFlareSuite) TestFlareDefaultFiles() {
	v.UpdateEnv(e2e.FakeIntakeStackDef(nil))
	flare := waitForAgentAndGetFlare(v, client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

	assertFilesExist(v.T(), flare, defaultFlareFiles)
	assertFilesExist(v.T(), flare, defaultLogFiles)
	assertFilesExist(v.T(), flare, defaultConfigFiles)
	assertFoldersExist(v.T(), flare, defaultFlareFolders)

	assertFileContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
	assertFileNotContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
	assertFileNotContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")

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
		"telemetry.log",       // if telemetry.enabled
		"registry.json",       // if Logs Agent is running
		"expvar/system-probe", // if system probe is enabled
	}

	// XXX: use FileManager here instead
	// XXX: this test is expected to fail because 'etc/security-agent.yaml' is not found. See #18463
	// TODO: use dedicated functions when https://github.com/DataDog/test-infra-definitions/pull/309 is merged
	v.Env().VM.Execute(fmt.Sprintf(`echo "%s" | sudo tee /etc/datadog-agent/system-probe.yaml`, systemProbeConfiguration))
	v.Env().VM.Execute(fmt.Sprintf(`echo "%s" | sudo tee /etc/datadog-agent/security-agent.yaml`, securityAgentConfiguration))
	v.Env().VM.Execute("sudo mkdir -p /tmp/dummy_dir /tmp/dummy_system_probe_config_bpf_dir")

	confdPath := "/opt/datadog-agent/bin/agent/dist/conf.d/"

	withFiles := []agentparams.Option{
		agentparams.WithFile(confdPath+"test.yaml", "dummy content", true),
		agentparams.WithFile(confdPath+"test.yml", "dummy content", true),
		agentparams.WithFile(confdPath+"test.yml.test", "dummy content", true),
		agentparams.WithFile("/opt/datadog-agent/checks.d/test.yml", "dummy content", true),
	}

	agentOptions := append(withFiles, agentparams.WithAgentConfig(string(agentConfiguration)))

	v.UpdateEnv(e2e.FakeIntakeStackDef(nil, agentOptions...))

	flare := waitForAgentAndGetFlare(v, client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

	assertFilesExist(v.T(), flare, scenarioExpectedFiles)
	assertFilesExist(v.T(), flare, allLogFiles)
	assertFilesExist(v.T(), flare, allConfigFiles)

	extraCustomConfigFiles := []string{"etc/confd/dist/test.yaml", "etc/confd/dist/test.yml", "etc/confd/dist/test.yml.test", "etc/confd/checksd/test.yml"}
	assertFilesExist(v.T(), flare, extraCustomConfigFiles)

	assertFileNotContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
	assertFileContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
	assertFileContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")

	filesRegistredInPermissionsLog := []string{"/etc/datadog-agent/auth_token", "/tmp/dummy_system_probe_config_bpf_dir/", "/tmp/dummy_dir"}
	assertFileContains(v.T(), flare, "permissions.log", filesRegistredInPermissionsLog...)
}
