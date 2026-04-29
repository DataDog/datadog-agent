// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrationslogs

import (
	_ "embed"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type IntegrationsLogsSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/tenLogs.py
var writeTenLogsCheck string

//go:embed fixtures/tenLogs.yaml
var writeTenLogsConfig string

// TestLinuxFakeIntakeSuite
func TestIntegrationsLogsSuite(t *testing.T) {
	// Provisioner creates infrastructure only — VM + fakeintake, no agent.
	e2e.Run(t, &IntegrationsLogsSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(awshost.WithRunOptions(scenec2.WithoutAgent())),
	))
}

func (v *IntegrationsLogsSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()
	hostagent.Install(v.T(), v.Env(),
		agentparams.WithLogs(),
		// set the integration log file max size to 1MB
		agentparams.WithAgentConfig("logs_config.integrations_logs_files_max_size: 1"),
		agentparams.WithFile("/etc/datadog-agent/checks.d/writeTenLogs.py", writeTenLogsCheck, true),
		agentparams.WithFile("/etc/datadog-agent/conf.d/writeTenLogs.yaml", writeTenLogsConfig, true),
	)
}

// TestWriteTenLogsCheck ensures a check that logs are written to the file ten
// logs at a time
func (v *IntegrationsLogsSuite) TestWriteTenLogsCheck() {
	utils.CheckLogsExpected(v.T(), v.Env().FakeIntake, "ten_logs_service", "Custom log message", []string{"env:dev", "bar:foo"})
}
