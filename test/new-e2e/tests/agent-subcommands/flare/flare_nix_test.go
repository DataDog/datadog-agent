// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package flare

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

//go:embed fixtures/datadog-agent.yaml
var agentConfiguration []byte

//go:embed fixtures/system-probe.yaml
var systemProbeConfiguration []byte

//go:embed fixtures/security-agent.yaml
var securityAgentConfiguration []byte

type linuxFlareSuite struct {
	baseFlareSuite
}

func TestLinuxFlareSuite(t *testing.T) {
	e2e.Run(t, &linuxFlareSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// Add zz to name to run this test last in order to don't break other tests
// Will need to rename it to TestFlareWithAllConfiguration after the fix of Paola's PR
// To keep in mind that we will need to create the directory then create file in it and specify to delete file as the same time as the directory
func (v *linuxFlareSuite) TestzzzFlareWithAllConfiguration() {
	scenarioExpectedFiles := []string{
		"telemetry.log",       // if telemetry.enabled
		"registry.json",       // if Logs Agent is running
		"expvar/system-probe", // if system probe is enabled
	}

	systemProbeDummyFiles := []string{"/tmp/dummy_dir", "/tmp/dummy_system_probe_config_bpf_dir"}
	v.Env().RemoteHost.MustExecute("sudo mkdir -p " + strings.Join(systemProbeDummyFiles, " "))

	confdPath := "/opt/datadog-agent/bin/agent/dist/conf.d/"
	useSudo := true

	withFiles := []agentparams.Option{
		agentparams.WithSystemProbeConfig(string(systemProbeConfiguration)),
		agentparams.WithSecurityAgentConfig(string(securityAgentConfiguration)),
		agentparams.WithFile(confdPath+"test.yaml", "dummy content", useSudo),
		agentparams.WithFile(confdPath+"test.yml", "dummy content", useSudo),
		agentparams.WithFile(confdPath+"test.yml.test", "dummy content", useSudo),
		agentparams.WithFile("/opt/datadog-agent/checks.d/test.yml", "dummy content", useSudo),
	}

	agentOptions := append(withFiles, agentparams.WithAgentConfig(string(agentConfiguration)))

	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentOptions...)))

	flare, _ := requestAgentFlareAndFetchFromFakeIntake(&v.baseFlareSuite, agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

	assertFilesExist(v.T(), flare, scenarioExpectedFiles)
	assertFilesExist(v.T(), flare, allLogFiles)
	assertFilesExist(v.T(), flare, allConfigFiles)

	extraCustomConfigFiles := []string{"etc/confd/dist/test.yaml", "etc/confd/dist/test.yml", "etc/confd/dist/test.yml.test", "etc/confd/checksd/test.yml"}
	assertFilesExist(v.T(), flare, extraCustomConfigFiles)

	assertFileNotContains(v.T(), flare, "process_check_output.json", "'process_config.process_collection.enabled' is disabled")
	assertFileContains(v.T(), flare, "container_check_output.json", "'process_config.container_collection.enabled' is disabled")
	assertFileContains(v.T(), flare, "process_discovery_check_output.json", "'process_config.process_discovery.enabled' is disabled")

	filesRegistredInPermissionsLog := append(systemProbeDummyFiles, "/etc/datadog-agent/auth_token")
	assertFileContains(v.T(), flare, "permissions.log", filesRegistredInPermissionsLog...)
}
