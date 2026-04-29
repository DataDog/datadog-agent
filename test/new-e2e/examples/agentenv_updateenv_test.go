// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"

	"github.com/stretchr/testify/assert"
)

type agentSuiteEx4 struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMSuiteEx4(t *testing.T) {
	// Provisioner creates infrastructure only — VM, no agent, no fakeintake.
	e2e.Run(t, &agentSuiteEx4{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(scenec2.WithoutAgent())),
	))
}

func (v *agentSuiteEx4) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	// Install the agent on the provisioned host via SSH and configure it.
	// This is fully decoupled from Pulumi.
	hostagent.Install(v.T(), v.Env(),
		agentparams.WithAgentConfig("log_level: debug"),
	)
}

func (v *agentSuiteEx4) TestLogDebug() {
	assert.Contains(v.T(), v.Env().Agent.Client.Config(), "log_level: debug")
}

func (v *agentSuiteEx4) TestLogInfo() {
	// Configure merges with baseline from Install — only changes log_level.
	v.Env().Agent.Configure(v.T(),
		agentparams.WithAgentConfig("log_level: info"),
	)
	assert.Contains(v.T(), v.Env().Agent.Client.Config(), "log_level: info")
}
