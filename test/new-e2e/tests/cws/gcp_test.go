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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	gcphost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/gcp/host/linux"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/config"
)

const (
	// gcpHostnamePrefix is the prefix of the hostname of the agent
	gcpHostnamePrefix = "cws-e2e-gcp-host"
)

func TestAgentSuiteGCP(t *testing.T) {
	testID := uuid.NewString()[:4]
	ddHostname := fmt.Sprintf("%s-%s", gcpHostnamePrefix, testID)
	agentConfig := config.GenDatadogAgentConfig(ddHostname, "tag1", "tag2")
	t.Logf("Running testsuite with DD_HOSTNAME=%s", ddHostname)
	e2e.Run[environments.Host](t, &agentSuite{testID: testID},
		e2e.WithStackName("cws-agentSuite-gcp"),
		e2e.WithProvisioner(
			gcphost.ProvisionerNoFakeIntake(
				gcphost.WithAgentOptions(

					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithSecurityAgentConfig(securityAgentConfig),
					agentparams.WithSystemProbeConfig(systemProbeConfig),
				),
			),
		),
	)
}
