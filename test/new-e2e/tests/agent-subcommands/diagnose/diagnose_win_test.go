// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains helpers and e2e tests of the flare command
package diagnose

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsDiagnoseSuite struct {
	baseDiagnoseSuite
}

func TestWindowsDiagnoseSuite(t *testing.T) {
	t.Parallel()
	var suite windowsDiagnoseSuite
	suite.suites = append(suite.suites, commonSuites...)
	e2e.Run(t, &suite, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))))))
}

func (v *windowsDiagnoseSuite) TestDiagnoseOtherCmdPort() {
	params := agentparams.WithAgentConfig("cmd_port: 4567")
	v.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)), ec2.WithAgentOptions(params))))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	v.AssertOutputNotError(diagnose)
}

func (v *windowsDiagnoseSuite) TestDiagnoseInclude() {
	v.AssertDiagnoseInclude()
	v.AssertDiagnoseJSONInclude()
}

func (v *windowsDiagnoseSuite) TestDiagnoseExclude() {
	v.AssertDiagnoseExclude()
	v.AssertDiagnoseJSONExclude()
}
