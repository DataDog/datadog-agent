// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
)

type linuxDiagnoseSuite struct {
	baseDiagnoseSuite
}

func TestLinuxDiagnoseSuite(t *testing.T) {
	t.Parallel()
	var suite linuxDiagnoseSuite
	suite.suites = append(suite.suites, commonSuites...)
	e2e.Run(t, &suite, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *linuxDiagnoseSuite) TestDiagnoseOtherCmdPort() {
	e2e.SetAgentConfig(v.T(), v.Env().RemoteHost, v.Env().Agent.Client,
		agentparams.WithAgentConfig("cmd_port: 4567"),
	)

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	v.AssertOutputNotError(diagnose)
}

func (v *linuxDiagnoseSuite) TestDiagnoseLocalFallback() {
	svcManager := svcmanager.NewSystemctl(v.Env().RemoteHost)
	svcManager.Stop("datadog-agent")

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	assert.Contains(v.T(), diagnose, "Running diagnose command locally", "Expected diagnose command to fallback to local diagnosis when the Agent is stopped, but it did not.")
	v.AssertOutputNotError(diagnose)

	svcManager.Start("datadog-agent")
}

func (v *linuxDiagnoseSuite) TestDiagnoseInclude() {
	v.AssertDiagnoseInclude()
	v.AssertDiagnoseJSONInclude()
}

func (v *linuxDiagnoseSuite) TestDiagnoseExclude() {
	v.AssertDiagnoseExclude()
	v.AssertDiagnoseJSONExclude()
}
