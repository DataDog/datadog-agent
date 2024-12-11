// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
)

type agentSuiteEx4 struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMSuiteEx4(t *testing.T) {
	e2e.Run(t, &agentSuiteEx4{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: debug")),
	)))
}

func (v *agentSuiteEx4) TestLogDebug() {
	assert.Contains(v.T(), v.Env().Agent.Client.Config(), "log_level: debug")
}

func (v *agentSuiteEx4) TestLogInfo() {
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(agentparams.WithAgentConfig("log_level: info")),
	))
	assert.Contains(v.T(), v.Env().Agent.Client.Config(), "log_level: info")
}
