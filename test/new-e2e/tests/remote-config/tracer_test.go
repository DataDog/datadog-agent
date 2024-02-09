// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

type tracerSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/rc-enabled.yaml
var rcEnabledConfig string

//go:embed fixtures/tracer-payload.json
var tracerPayloadJSON string

func TestRcTracerSuite(t *testing.T) {
	e2e.Run(t, &tracerSuite{},
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithAgentOptions(
					agentparams.WithAgentConfig(rcEnabledConfig),
				),
			),
		),
	)
}

// TestRemoteConfigTracerUpdate tests the remote-config service by attempting to retrieve RC payloads as if a tracer were calling it
func (s *tracerSuite) TestRemoteConfigTracerUpdate() {
	// Ensure the remote config service starts
	// TODO uncomment the following line in https://github.com/DataDog/datadog-agent/pull/22582 (once fx lifecycle startup logging is added)
	//assertLogsEventually(a.T(), a.Env().RemoteHost, "agent", "remote config service started", 2*time.Minute, 5*time.Second)

	// Wait until we've started querying for configs
	assertLogsEventually(s.T(), s.Env().RemoteHost, "agent", "/api/v0.1/configurations", 2*time.Minute, 5*time.Second)

	// Get configs as though we are a tracer
	// But first, prime by continuously curling until the api is responding successfully just in case it is slow to start
	getConfigsOutput := mustCurlAgentRcServiceEventually(s.T(), s.Env().RemoteHost, tracerPayloadJSON, 2*time.Minute, 5*time.Second)
	require.Contains(s.T(), getConfigsOutput, "roots", "expected a roots key in the tracer config output")
	require.Contains(s.T(), getConfigsOutput, "targets", "expected a targets key in the tracer config output")

	// Check remote-config command output for our e2e test client that we fetched configs for
	remoteConfigOutput := s.Env().Agent.Client.RemoteConfig()
	require.Contains(s.T(), remoteConfigOutput, "=== Remote config DB state ===", "could not find the expected header in the core agent remote-config output")
	require.Contains(s.T(), remoteConfigOutput, "Client e2e_tests", "could not find the e2e_tests client in the core agent remote-config output")
}
