// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"encoding/json"
	"strings"
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
//
// This test validates that:
// 1. The process listener detects the redis-server process
// 2. The cel_selector in the check config matches the process
// 3. The check is scheduled with a process:// service ID (not just from file)
// 4. The check runs successfully
func (s *processAutodiscoverySuite) TestRedisCheckScheduledViaProcessAutodiscovery() {
	t := s.T()

	// Wait for the agent to discover the process and schedule the check
	// Process discovery runs on an interval, so we need to wait for:
	// 1. Process to be detected and added to workloadmeta
	// 2. Process listener to receive the event and create a service
	// 3. Autodiscovery to match the service with the redis check config
	// 4. Check to be scheduled and run
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyRedisCheckScheduledViaProcess(c)
	}, 3*time.Minute, 10*time.Second, "Redis check should be scheduled via process autodiscovery")
}

// verifyRedisCheckScheduledViaProcess verifies that the redisdb check is scheduled
// AND that it was scheduled via process autodiscovery (not just loaded from file)
func (s *processAutodiscoverySuite) verifyRedisCheckScheduledViaProcess(c *assert.CollectT) {
	t := s.T()

	statusOutput := s.Env().RemoteHost.MustExecute("sudo datadog-agent status collector --json")

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if !assert.NoError(c, err, "failed to parse agent status") {
		t.Logf("Failed to parse status output: %s", statusOutput)
		return
	}

	// Check if redisdb check is in the scheduled checks
	instances, exists := status.RunnerStats.Checks["redisdb"]
	if !assert.True(c, exists, "redisdb check should be scheduled") {
		t.Logf("Available checks: %v", getCheckNames(status.RunnerStats.Checks))
		return
	}

	// Verify the check was scheduled via process autodiscovery
	// The CheckConfigSource should contain "process://" indicating it was
	// resolved against a process service, not just loaded from file
	foundProcessService := false
	for instanceName, checkStat := range instances {
		t.Logf("Redis check instance %s: runs=%d, source=%s",
			instanceName, len(checkStat.ExecutionTimes), checkStat.CheckConfigSource)

		// Check if this instance was scheduled via process autodiscovery
		// The source should indicate it came from a process service
		if strings.Contains(checkStat.CheckConfigSource, "process://") {
			foundProcessService = true
			if len(checkStat.ExecutionTimes) > 0 {
				t.Log("Redis check is scheduled via process autodiscovery and has run successfully")
				return
			}
		}
	}

	if !foundProcessService {
		assert.Fail(c, "Redis check was not scheduled via process autodiscovery. "+
			"The check source should contain 'process://' but found only file-based configs. "+
			"This indicates the process listener is not working correctly.")
		return
	}

	assert.Fail(c, "Redis check is scheduled via process autodiscovery but has not run yet")
}

// getCheckNames returns the names of all scheduled checks
func getCheckNames(checks map[checkName]map[instanceName]checkStatus) []string {
	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}
	return names
}
