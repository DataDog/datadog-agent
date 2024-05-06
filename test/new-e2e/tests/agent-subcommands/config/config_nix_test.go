// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains helpers and e2e tests for config subcommand
package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type linuxConfigSuite struct {
	baseConfigSuite
}

func TestLinuxConfigSuite(t *testing.T) {
	osOption := awshost.WithEC2InstanceOptions(ec2.WithOS(os.UbuntuDefault))
	e2e.Run(t, &linuxConfigSuite{baseConfigSuite: baseConfigSuite{osOption: osOption}}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}
