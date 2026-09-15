// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentmetricemission verifies the Agent is emitting its heartbeat
// metric. The same test body runs against a remote EC2 host (the standard
// Pulumi provisioner) and against a local Docker container agent (the
// e2ectl `local` environment), proving the transport is transparent.
package agentmetricemission

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// metricSuite runs the same assertions regardless of where the Agent lives.
type metricSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestMetricEmissionOnHost is the standard path: a provisioned EC2 VM
// running the released Agent.
func TestMetricEmissionOnHost(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &metricSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// TestMetricEmissionOnLocal runs the exact same test body against a local
// Docker container agent managed by e2ectl. The environment must be running
// (e2ectl start) and the agent installed (e2ectl install) before the test
// starts; the test attaches via the snapshot.
func TestMetricEmissionOnLocal(t *testing.T) {
	envName := os.Getenv("E2ECTL_LOCAL_ENV")
	if envName == "" {
		t.Skip("set E2ECTL_LOCAL_ENV to a running e2ectl local environment to run this test")
	}
	home := os.Getenv("E2ECTL_HOME")
	if home == "" {
		home = os.ExpandEnv("$HOME/.e2ectl")
	}
	snapshot := filepath.Join(home, "envs", envName, "snapshot.json")
	if _, err := os.Stat(snapshot); err != nil {
		t.Skipf("no snapshot for %s (e2ectl start first): %v", envName, err)
	}
	t.Parallel()
	e2e.Run(t, &metricSuite{}, e2e.WithProvisioner(
		provisioners.NewStaticStackProvisioner[environments.Host]("local-attach", snapshot),
	))
}

// TestAgentHeartbeat verifies the Agent is alive and flushing to the
// fakeintake — the same assertion on a VM or in a container.
func (s *metricSuite) TestAgentHeartbeat() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.agent.running")
		require.NoError(c, err, "failed to filter metrics")
		assert.NotEmpty(c, metrics, "no heartbeat metric yet")
	}, 2*time.Minute, 15*time.Second, "agent heartbeat not found in fakeintake")
}

// TestAgentCommandExecution verifies RemoteHost.Execute works — on an EC2
// VM via SSH, in a local container via docker exec. The same interface.
func (s *metricSuite) TestAgentCommandExecution() {
	output := s.Env().RemoteHost.MustExecute("cat /etc/datadog-agent/datadog.yaml")
	s.Assert().Contains(output, "dd_url:", "the agent config should contain dd_url")
}

// TestFakeintakeReachable verifies the fakeintake is queryable from the test.
func (s *metricSuite) TestFakeintakeReachable() {
	err := s.Env().FakeIntake.Client().GetServerHealth()
	s.Require().NoError(err, "fakeintake should be reachable")
}

// TestCpuCheckIsRunning verifies a default Go core check is configured and
// running — proving the default core-check seeding works on both paths.
func (s *metricSuite) TestCpuCheckIsRunning() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.cpu.user")
		require.NoError(c, err, "failed to filter cpu metrics")
		assert.NotEmpty(c, metrics, "no system.cpu.user metric yet")
	}, 2*time.Minute, 15*time.Second, "cpu check not emitting metrics")
}
