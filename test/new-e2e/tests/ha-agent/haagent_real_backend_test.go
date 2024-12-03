// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package haagent contains e2e tests for HA Agent feature
package haagent

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type haAgentRealBackendTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestHaAgentRealBackendSuite runs the HA Agent e2e suite
func TestHaAgentRealBackendSuite(t *testing.T) {
	apiKey := os.Getenv("DD_API_KEY") // Used for local testing
	if apiKey == "" {
		apiKeyFromSecreteStore, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
		require.NoError(t, err, "Could not get API KEY")
		apiKey = apiKeyFromSecreteStore
	}

	if apiKey != "" {
		t.Logf("Using API_KEY ending with: %s", apiKey[len(apiKey)-4:])
	} else {
		require.Fail(t, "API_KEY is empty")
	}

	// language=yaml
	agentConfig := fmt.Sprintf(`
ha_agent:
    enabled: true
    group: e2e-test-%s
log_level: debug
api_key: %s
site: datad0g.com
`, uuid.New().String(), apiKey)

	// language=yaml
	snmpIntegration := `
init_config:
  loader: core
  use_device_id_as_hostname: true
instances:
- ip_address: '1.2.3.4'
  community_string: 'sample-string'
`

	e2e.Run(t, &haAgentRealBackendTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithSkipAPIKeyInConfig(),
			agentparams.WithIntegration("snmp.d", snmpIntegration),
		),
		awshost.WithoutFakeIntake(),
	),
	))
}

func (s *haAgentRealBackendTestSuite) TestSnmpCheckIsRunningOnLeaderAgent() {
	snmpCheckSkippedLog := "check:snmp | Check is an HA integration and current agent is not leader, skipping execution..."
	snmpCheckRunningLog := "check:snmp | Running check..."

	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert snmp check is running")
		output, err := s.Env().RemoteHost.Execute("cat /var/log/datadog/agent.log")
		if !assert.NoError(c, err) {
			return
		}

		assert.Contains(c, output, snmpCheckSkippedLog)
		assert.Contains(c, output, snmpCheckRunningLog)

		// Assert snmp check was first skipped, then running
		assert.Greater(c, strings.Index(output, snmpCheckRunningLog), strings.Index(output, snmpCheckSkippedLog))
	}, 5*time.Minute, 3*time.Second)
}
