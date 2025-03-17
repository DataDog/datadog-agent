// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cws holds cws e2e tests
package cws

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/config"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

const (
	// ec2HostnamePrefix is the prefix of the hostname of the agent
	ec2HostnamePrefix = "cws-e2e-ec2-host"
)

func TestAgentSuiteEC2(t *testing.T) {
	testID := uuid.NewString()[:4]
	ddHostname := fmt.Sprintf("%s-%s", ec2HostnamePrefix, testID)
	agentConfig := config.GenDatadogAgentConfig(ddHostname, "tag1", "tag2")
	e2e.Run[environments.Host](t, &agentSuite{testID: testID},
		e2e.WithStackName("cws-agentSuite-ec2"),
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithSecurityAgentConfig(securityAgentConfig),
					agentparams.WithSystemProbeConfig(systemProbeConfig),
				),
			),
		),
	)
	t.Logf("Running testsuite with DD_HOSTNAME=%s", ddHostname)
}
