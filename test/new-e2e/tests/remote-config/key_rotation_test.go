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

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type keyRotationSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/tracer-payload.json
var tracerPayloadJSON string

// TestRcKeyRotationSuite asserts that rotating RC keys triggers the failsafe that
// clears the local config cache
func TestRcKeyRotationSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &keyRotationSuite{},
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithAgentOptions(
					agentparams.WithAgentConfig(rcEnabledConfig),
				),
			),
		),
	)
}

// TestRemoteConfigKeyRotation asserts that rotating RC keys triggers the failsafe that
// clears the local config cache
func (s *keyRotationSuite) TestRemoteConfigKeyRotation() {
	expectedAgentLogs := []string{
		// Ensure the remote config service starts
		"remote config service started",
	}
	assertAgentLogsEventually(s.T(), s.Env().RemoteHost, "agent", expectedAgentLogs, 2*time.Minute, 5*time.Second)

	// Get configs as though we are a tracer
	expectedKeys := []string{
		"roots",
		"targets",
	}
	assertCurlAgentRcServiceContainsEventually(s.T(), s.Env().RemoteHost, tracerPayloadJSON, expectedKeys, 2*time.Minute, 5*time.Second)

	// Check remote-config command output for our e2e test client that we fetched configs for
	remoteConfigOutput := s.Env().Agent.Client.RemoteConfig()
	require.Contains(s.T(), remoteConfigOutput, "=== Remote config DB state ===", "could not find the expected header in the core agent remote-config output")
	require.Contains(s.T(), remoteConfigOutput, "Client e2e_tests", "could not find the e2e_tests client in the core agent remote-config output")
}
