// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	_ "embed"
	"testing"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

type winFlareSuite struct {
	commandFlareSuite
}

// you can use an environment variable or a file
const isLocalRun = false

var (
	localVM    client.VM
	localAgent client.Agent
)

// getVM returns a vm client that can execute commands
// TODO Can go in some common helpers
func (s *winFlareSuite) getVM() client.VM {
	if isLocalRun {
		return localVM
	}
	return s.Env().VM
}

func (s *winFlareSuite) getAgent() client.Agent {
	if isLocalRun {
		return localAgent
	}
	return s.Env().Agent
}

func (s *winFlareSuite) getFakeintake() *fakeintake.Client {
	return s.Env().Fakeintake.Cliet
}

func TestFlareWinSuite(t *testing.T) {
	// TODO we could pass a different local stackDefinition with only fakeintake
	e2e.Run(t, &winFlareSuite{}, e2e.FakeIntakeStackDef(e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS))), params.WithLazyEnvironment())
}

func (v *winFlareSuite) TestFlareDefaultFiles() {
	flare := requestAgentFlareAndFetchFromFakeIntake(v.T(), v.getAgent(),, v.getFakeintake() client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

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

// //go:embed fixtures/datadog-agent.yaml
// var agentConfiguration []byte

// //go:embed fixtures/system-probe.yaml
// var systemProbeConfiguration []byte

// //go:embed fixtures/security-agent.yaml
// var securityAgentConfiguration []byte

// func (v *commandFlareSuite) TestFlareWithAllConfiguration() {

// 	var scenarioExpectedFiles = []string{
// 		"telemetry.log",       // if telemetry.enabled
// 		"registry.json",       // if Logs Agent is running
// 		"expvar/system-probe", // if system probe is enabled
// 	}

// 	systemProbeDummyFiles := []string{"/tmp/dummy_dir", "/tmp/dummy_system_probe_config_bpf_dir"}
// 	v.Env().VM.Execute("sudo mkdir -p " + strings.Join(systemProbeDummyFiles, " "))

// 	confdPath := "/opt/datadog-agent/bin/agent/dist/conf.d/"
// 	useSudo := true

// 	withFiles := []agentparams.Option{
// 		// TODO: use dedicated functions when https://github.com/DataDog/test-infra-definitions/pull/309 is merged
// 		agentparams.WithSystemProbeConfig(string(systemProbeConfiguration)),
// 		agentparams.WithSecurityAgentConfig(string(securityAgentConfiguration)),
// 		agentparams.WithFile(confdPath+"test.yaml", "dummy content", useSudo),
// 		agentparams.WithFile(confdPath+"test.yml", "dummy content", useSudo),
// 		agentparams.WithFile(confdPath+"test.yml.test", "dummy content", useSudo),
// 		agentparams.WithFile("/opt/datadog-agent/checks.d/test.yml", "dummy content", useSudo),
// 	}

// 	agentOptions := append(withFiles, agentparams.WithAgentConfig(string(agentConfiguration)))

// 	v.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(agentOptions...)))

// 	flare := requestAgentFlareAndFetchFromFakeIntake(v, client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

// 	assertFilesExist(v.T(), flare, scenarioExpectedFiles)
// 	assertFilesExist(v.T(), flare, allLogFiles)
// 	// XXX: this test is expected to fail because 'etc/security-agent.yaml' is not found. See #18463
// 	assertFilesExist(v.T(), flare, allConfigFiles)

// 	extraCustomConfigFiles := []string{"etc/confd/dist/test.yaml", "etc/confd/dist/test.yml", "etc/confd/dist/test.yml.test", "etc/confd/checksd/test.yml"}
// 	assertFilesExist(v.T(), flare, extraCustomConfigFiles)

// 	assertFileNotContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
// 	assertFileContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
// 	assertFileContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")

// 	filesRegistredInPermissionsLog := append(systemProbeDummyFiles, "/etc/datadog-agent/auth_token")
// 	assertFileContains(v.T(), flare, "permissions.log", filesRegistredInPermissionsLog...)
// }
