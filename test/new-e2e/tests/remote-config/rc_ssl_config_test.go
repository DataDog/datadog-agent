// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	_ "embed"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

type sslConfigSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/ssl_mismatch.yaml
var sslMismatchConfig string

func TestSslConfigSuite(t *testing.T) {
	e2e.Run(t, &sslConfigSuite{},
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithAgentOptions(
					agentparams.WithAgentConfig(sslMismatchConfig),
				),
			),
		),
	)
}

// TestRemoteConfigSSLConfigMismatch tests the startup condition where the agent's SSL config is disabled but RC's TLS validation is not explicitly disabled
func (s *sslConfigSuite) TestRemoteConfigSSLConfigMismatch() {
	// Check if the agent is ready
	isReady := s.Env().Agent.Client.IsReady()
	assert.Equal(s.T(), isReady, true, "Agent is not ready")

	// Ensure the remote config service starts
	// TODO uncomment the following line in https://github.com/DataDog/datadog-agent/pull/22582 (once fx lifecycle startup logging is added)
	//assertLogsWithRetry(a.T(), a.Env().RemoteHost, "agent", "remote config service started", 60, 500*time.Millisecond)

	// Ensure the agent logs a warning about the SSL config mismatch
	assertLogsWithRetry(s.T(), s.Env().RemoteHost, "agent", "remote Configuration does not allow skipping TLS validation by default", 120, 1*time.Second)
	// Ensure the remote config service stops, and the client stops because the service is not longer responding
	assertLogsWithRetry(s.T(), s.Env().RemoteHost, "agent", "remote configuration isn't enabled, disabling client", 120, 1*time.Second)

	// Ensure the agent remains running despite the remote config service initialization failure
	isReady = s.Env().Agent.Client.IsReady()
	assert.Equal(s.T(), isReady, true, "Agent shut down after remote config initialization failed")
}
