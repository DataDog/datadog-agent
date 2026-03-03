// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains helpers and e2e tests for config subcommand
package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsConfigSuite struct {
	baseConfigSuite
}

func TestWindowsConfigSuite(t *testing.T) {
	osOption := scenec2.WithEC2InstanceOptions(scenec2.WithOS(os.WindowsServerDefault))
	t.Parallel()
	e2e.Run(t, &windowsConfigSuite{baseConfigSuite: baseConfigSuite{osOption: osOption}}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithRunOptions(
			scenec2.WithEC2InstanceOptions(scenec2.WithOS(os.WindowsServerDefault)),
		),
	)))
}
