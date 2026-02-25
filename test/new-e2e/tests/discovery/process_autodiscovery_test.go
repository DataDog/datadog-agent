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

//go:embed testdata/config/nginx_process_autodiscovery.yaml
var nginxProcessAutodiscoveryConfigStr string

type processAutodiscoverySuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestProcessAutodiscoverySuite(t *testing.T) {
	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(agentProcessAutodiscoveryConfigStr),
		agentparams.WithSystemProbeConfig(processAutodiscoverySystemProbeConfigStr),
		// Add the redis check configuration with cel_selector for process-based autodiscovery
		agentparams.WithIntegration("redisdb.d", redisProcessAutodiscoveryConfigStr),
		// Add the nginx check configuration with cel_selector for process-based autodiscovery
		agentparams.WithIntegration("nginx.d", nginxProcessAutodiscoveryConfigStr),
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

	// Install nginx
	_, err = s.Env().RemoteHost.Execute("sudo apt-get install -y nginx")
	require.NoError(s.T(), err, "failed to install nginx")

	// Configure nginx with multiple workers and stub_status on port 81
	nginxConf := `worker_processes 4;
events {}
http {
    server {
        listen 81;
        location /nginx_status {
            stub_status;
        }
    }
}
`
	s.Env().RemoteHost.MustExecute("echo '" + nginxConf + "' | sudo tee /etc/nginx/nginx.conf")
	s.Env().RemoteHost.MustExecute("sudo nginx -t && sudo systemctl reload nginx")

	// Verify nginx stub_status is accessible, retrying to allow reload to complete
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute("curl -s http://localhost:81/nginx_status")
		assert.NoError(c, err, "curl failed")
		assert.Contains(c, output, "Active connections", "nginx stub_status should be accessible")
	}, 30*time.Second, 2*time.Second)
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

	require.Contains(c, configCheckOutput, "=== redisdb check ===", "redisdb check should be configured")

	// Verify the check has cel://process AD identifier
	require.Contains(c, configCheckOutput, "cel://process", "redisdb check should have cel://process AD identifier")

	// Verify host was resolved to localhost
	require.Contains(c, configCheckOutput, "host: 127.0.0.1", "redisdb check should have host resolved to 127.0.0.1")

	// Verify the check is running via collector status
	statusOutput := s.Env().RemoteHost.MustExecute("sudo datadog-agent status collector --json")

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	require.NoError(c, err, "failed to parse agent status")

	instances, exists := status.RunnerStats.Checks["redisdb"]
	require.True(c, exists, "redisdb check should be running")

	// Verify the check has executed successfully
	for instanceName, checkStat := range instances {
		if len(checkStat.ExecutionTimes) > 0 {
			t.Logf("Redis check instance %s: runs=%d", instanceName, len(checkStat.ExecutionTimes))
			return
		}
	}

	assert.Fail(c, "Redis check is configured but has not run yet")
}

// TestNginxCheckScheduledViaProcessAutodiscovery verifies that the nginx check
// is automatically scheduled when an nginx process is detected via
// the process autodiscovery listener (cel://process), and that exactly one
// check instance runs despite multiple nginx worker processes.
func (s *processAutodiscoverySuite) TestNginxCheckScheduledViaProcessAutodiscovery() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyNginxCheckScheduledViaProcess(c)
	}, 3*time.Minute, 10*time.Second, "Nginx check should be scheduled via process autodiscovery")
}

// verifyNginxCheckScheduledViaProcess verifies that the nginx check is scheduled
// via process autodiscovery with exactly one instance despite multiple nginx processes
func (s *processAutodiscoverySuite) verifyNginxCheckScheduledViaProcess(c *assert.CollectT) {
	t := s.T()

	// Log nginx process count for debugging
	nginxCount, _ := s.Env().RemoteHost.Execute("pgrep -c nginx")
	t.Logf("nginx process count: %s", nginxCount)

	// Verify check configuration via config-check
	configCheckOutput := s.Env().RemoteHost.MustExecute("sudo datadog-agent configcheck")

	require.Contains(c, configCheckOutput, "=== nginx check ===", "nginx check should be configured")

	// Verify the check has cel://process AD identifier
	require.Contains(c, configCheckOutput, "cel://process", "nginx check should have cel://process AD identifier")

	// Verify the check is running via collector status
	statusOutput := s.Env().RemoteHost.MustExecute("sudo datadog-agent status collector --json")

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	require.NoError(c, err, "failed to parse agent status")

	instances, exists := status.RunnerStats.Checks["nginx"]
	require.True(c, exists, "nginx check should be running")

	// Key assertion: exactly 1 nginx check instance despite multiple nginx processes
	assert.Equal(c, 1, len(instances), "expected exactly 1 nginx check instance, got %d", len(instances))

	// Verify the check has executed successfully
	for instanceName, checkStat := range instances {
		if len(checkStat.ExecutionTimes) > 0 {
			t.Logf("Nginx check instance %s: runs=%d", instanceName, len(checkStat.ExecutionTimes))
			return
		}
	}

	assert.Fail(c, "Nginx check is configured but has not run yet")
}

// getCheckNames returns the names of all scheduled checks
func getCheckNames(checks map[checkName]map[instanceName]checkStatus) []string {
	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}
	return names
}
