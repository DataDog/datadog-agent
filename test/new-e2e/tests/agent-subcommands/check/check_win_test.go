// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains helpers and e2e tests of the check command
package check

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type windowsCheckSuite struct {
	baseCheckSuite
}

func TestWindowsCheckSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsCheckSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(
				agentparams.WithFile("C:/ProgramData/Datadog/conf.d/hello.d/conf.yaml", string(customCheckYaml), true),
				agentparams.WithFile("C:/ProgramData/Datadog/checks.d/hello.py", string(customCheckPython), true),
			),
		),
	))
}
