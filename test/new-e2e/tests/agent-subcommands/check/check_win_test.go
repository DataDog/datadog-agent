// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains helpers and e2e tests of the check command
package check

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsCheckSuite struct {
	baseCheckSuite
}

func TestWindowsCheckSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsCheckSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithEC2InstanceOptions(scenec2.WithOS(os.WindowsServerDefault)),
				scenec2.WithAgentOptions(
					agentparams.WithIntegration("hello.d", string(customCheckYaml)),
					agentparams.WithFile("C:/ProgramData/Datadog/checks.d/hello.py", string(customCheckPython), true),
				),
			),
		)),
	)
}
