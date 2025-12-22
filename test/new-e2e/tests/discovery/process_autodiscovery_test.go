// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed testdata/config/agent_process_autodiscovery.yaml
var agentProcessAutodiscoveryConfigStr string

//go:embed testdata/config/system_probe_config.yaml
var processAutodiscoverySystemProbeConfigStr string

//go:embed testdata/config/redisdb_process_autodiscovery.yaml
var redisProcessAutodiscoveryConfigStr string

type processAutodiscoverySuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestProcessAutodiscoverySuite(t *testing.T) {
	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(agentProcessAutodiscoveryConfigStr),
		agentparams.WithSystemProbeConfig(processAutodiscoverySystemProbeConfigStr),
		// Add the redis check configuration with cel_selector for process-based autodiscovery
		agentparams.WithIntegration("redisdb.d", redisProcessAutodiscoveryConfigStr),
	}
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentParams...)))),
	}
	e2e.Run(t, &processAutodiscoverySuite{}, options...)
}

func (s *processAutodiscoverySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// Install Redis - it starts automatically and binds to localhost:6379 by default
	_, err := s.Env().RemoteHost.Execute("sudo apt-get update && sudo apt-get install -y redis-server")
	require.NoError(s.T(), err, "failed to install redis-server")

	// Verify Redis is running
	output := s.Env().RemoteHost.MustExecute("redis-cli ping")
	require.Contains(s.T(), output, "PONG", "Redis server should be running")
}

// TestRedisCheckScheduledViaProcessAutodiscovery verifies that the redis check
// is automatically scheduled when a redis-server process is detected via
// the process autodiscovery listener (cel://process).
func (s *processAutodiscoverySuite) TestRedisCheckScheduledViaProcessAutodiscovery() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyRedisCheckScheduledViaProcess(c)
	}, 3*time.Minute, 10*time.Second, "Redis check should be scheduled via process autodiscovery")
}

// verifyRedisCheckScheduledViaProcess verifies that the redisdb check is scheduled
// via process autodiscovery and running correctly
func (s *processAutodiscoverySuite) verifyRedisCheckScheduledViaProcess(c *assert.CollectT) {
	t := s.T()

	// Verify check configuration via config-check
	configCheckOutput := s.Env().RemoteHost.MustExecute("sudo datadog-agent configcheck")

	if !assert.Contains(c, configCheckOutput, "=== redisdb check ===", "redisdb check should be configured") {
		t.Logf("config-check output: %s", configCheckOutput)
		return
	}

	// Verify the check has cel://process AD identifier
	if !assert.Contains(c, configCheckOutput, "cel://process", "redisdb check should have cel://process AD identifier") {
		t.Logf("config-check output: %s", configCheckOutput)
		return
	}

	// Verify host was resolved to localhost
	if !assert.Contains(c, configCheckOutput, "host: 127.0.0.1", "redisdb check should have host resolved to 127.0.0.1") {
		t.Logf("config-check output: %s", configCheckOutput)
		return
	}

	// Verify the check is running via collector status
	statusOutput := s.Env().RemoteHost.MustExecute("sudo datadog-agent status collector --json")

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if !assert.NoError(c, err, "failed to parse agent status") {
		t.Logf("Failed to parse status output: %s", statusOutput)
		return
	}

	instances, exists := status.RunnerStats.Checks["redisdb"]
	if !assert.True(c, exists, "redisdb check should be running") {
		t.Logf("Available checks: %v", getCheckNames(status.RunnerStats.Checks))
		return
	}

	// Verify the check has executed successfully
	for instanceName, checkStat := range instances {
		if len(checkStat.ExecutionTimes) > 0 {
			t.Logf("Redis check instance %s: runs=%d", instanceName, len(checkStat.ExecutionTimes))
			return
		}
	}

	assert.Fail(c, "Redis check is configured but has not run yet")
}

// getCheckNames returns the names of all scheduled checks
func getCheckNames(checks map[checkName]map[instanceName]checkStatus) []string {
	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}
	return names
}
