// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// amdMandatoryMetricTags are the device tags every AMD GPU metric carries.
// gpu_driver_version is not one of them: the in-tree amdgpu module does not
// export a version.
var amdMandatoryMetricTags = []string{"gpu_uuid", "gpu_device", "gpu_vendor", "gpu_pci_bus_id"}

type amdGPUHostSuite struct {
	e2e.BaseSuite[environments.Host]
	agentRestartsAtSuiteInit int
}

// TestAMDGPUHostSuite runs the Agent on a host whose only GPU is an AMD GPU, to
// cover discovery from sysfs, the GPU check and payload delivery together.
// Not to be run in parallel, as some tests wait until the checks are available.
func TestAMDGPUHostSuite(t *testing.T) {
	flake.MarkOnLog(t, "error: an unhandled error occurred: waiting for RPCs:") // incident-33572

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(amdGPUHostProvisioner())}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}
	e2e.Run(t, &amdGPUHostSuite{}, suiteParams...)
}

func (s *amdGPUHostSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.agentRestartsAtSuiteInit = s.coreAgentRestartCount()
}

func (s *amdGPUHostSuite) coreAgentRestartCount() int {
	caps := &hostCapabilities{suite: &s.BaseSuite}
	return caps.GetRestartCount(agentComponentCoreAgent)
}

func (s *amdGPUHostSuite) TestGPUCheckIsEnabled() {
	// The GPU check is scheduled by autodiscovery, so it can take some time to run.
	s.EventuallyWithT(func(c *assert.CollectT) {
		statusOutput, err := s.Env().Agent.Client.StatusWithError(agentclient.WithArgs([]string{"collector", "--json"}))
		require.NoError(c, err, "failed to get agent collector status")

		// The JSON status is the second-to-last line, the rest is standard error.
		statusLines := strings.Split(statusOutput.Content, "\n")
		require.Greater(c, len(statusLines), 1, "status output should have at least 2 lines")

		var status collectorStatus
		require.NoError(c, json.Unmarshal([]byte(statusLines[len(statusLines)-2]), &status), "failed to unmarshal agent status")
		require.Contains(c, status.RunnerStats.Checks, "gpu", "gpu check should be enabled")

		gpuCheckStatus := status.RunnerStats.Checks["gpu"]
		s.T().Logf("gpu check status: %+v", gpuCheckStatus)
		require.NotEmpty(c, gpuCheckStatus.ExecutionTimes, "gpu check should have run")
		require.Empty(c, gpuCheckStatus.LastError)
	}, 5*time.Minute, 10*time.Second)
}

func (s *amdGPUHostSuite) TestDeviceMetricsAreReported() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		// Memory is reported by every amdgpu device, virtual functions included.
		// Activity, temperature and power depend on what the device exposes.
		for _, metricName := range []string{"gpu.memory.limit", "gpu.memory.free", "gpu.device.total"} {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName,
				client.WithTags[*aggregator.MetricSeries]([]string{"gpu_vendor:amd"}),
				client.WithMetricValueHigherThan(0))
			assert.NoError(c, err)
			if metricName == "gpu.device.total" {
				assert.NotEmpty(c, metrics, "no %s metric for AMD GPUs", metricName)
				continue
			}
			if !assertMetricsHaveExpectedTagKeys(c, metrics, amdMandatoryMetricTags, metricName) {
				continue
			}
			for _, metric := range metrics {
				assert.Contains(c, metric.GetTags(), "gpu_vendor:amd")
				for _, tag := range metric.GetTags() {
					if uuid, ok := strings.CutPrefix(tag, "gpu_uuid:"); ok {
						assert.True(c, strings.HasPrefix(uuid, "amd-"), "AMD GPU UUID should start with amd-, got %s", uuid)
					}
				}
			}
		}
	}, 5*time.Minute, 10*time.Second)
}

func (s *amdGPUHostSuite) TestWorkloadmetaHasAMDGPU() {
	var out string
	s.EventuallyWithT(func(c *assert.CollectT) {
		status, err := s.Env().Agent.Client.WorkloadList()
		assert.NoError(c, err)
		out = status.Content
		assert.Contains(c, out, "=== Entity gpu sources(merged):[amdgpu] id: amd-")
	}, 2*time.Minute, 5*time.Second)

	if s.T().Failed() {
		s.T().Log(out)
	}
}

// TestZZAgentDidNotRestart runs last to catch a crash after the metrics were sent.
func (s *amdGPUHostSuite) TestZZAgentDidNotRestart() {
	s.Assert().Equal(s.agentRestartsAtSuiteInit, s.coreAgentRestartCount(), "the core agent restarted during the test suite")
}
