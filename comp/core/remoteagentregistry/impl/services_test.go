// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	helpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

func TestGetRegisteredAgentStatuses(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		withStatusProvider(map[string]string{
			"test_key": "test_value",
		}, nil),
	)

	statuses := component.GetRegisteredAgentStatuses()
	t.Logf("statuses: %v\n", statuses[0])
	require.Len(t, statuses, 1)
	require.Equal(t, "test-agent", statuses[0].Flavor)
	require.Equal(t, "Test Agent", statuses[0].DisplayName)
	require.Equal(t, "test_value", statuses[0].MainSection["test_key"])
}

func TestFlareProvider(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp
	flareProvider := provides.FlareProvider

	remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		WithFlareProvider(map[string][]byte{
			"test_file.yaml": []byte("test_content"),
		}),
	)

	fb := helpers.NewFlareBuilderMock(t, false)
	fb.AssertNoFileExists("test-agent/test_file.yaml")

	err := flareProvider.FlareFiller.Callback(fb)
	require.NoError(t, err)

	flareFilePath := remoteAgent.RegistrationData.AgentDisplayName + "-" + remoteAgent.RegistrationData.AgentFlavor + "-" + remoteAgent.registeredSessionID + "/test_file.yaml"
	fb.AssertFileExists(flareFilePath)
	fb.AssertFileContent("test_content", flareFilePath)
}

func TestGetTelemetry(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	lc.Start(context.Background())
	component := provides.Comp

	promText := `
		# HELP foobar foobarhelp
		# TYPE foobar counter
		foobar 1
		# HELP baz bazhelp
		# TYPE baz gauge
		baz{tag_one="1",tag_two="two"} 3
		# HELP qux quxhelp
		# TYPE qux histogram
		qux_bucket{le="0.5"} 1
		qux_bucket{le="1"} 2
		qux_bucket{le="+Inf"} 3
		qux_sum 2.5
		qux_count 3
		`

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		withTelemetryProvider(promText),
	)

	metrics, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	// convert the metrics to a map for easier comparison
	metricsMap := make(map[string]*io_prometheus_client.MetricFamily)
	for _, m := range metrics {
		metricsMap[m.GetName()] = m
	}

	// compare the foobar metrics
	require.Contains(t, metricsMap, "foobar")
	assert.Empty(t, cmp.Diff(metricsMap["foobar"], &io_prometheus_client.MetricFamily{
		Name: proto.String("foobar"),
		Type: io_prometheus_client.MetricType_COUNTER.Enum(),
		Help: proto.String("foobarhelp"),
		Metric: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{
					{
						Name:  proto.String(remoteAgentMetricTagName),
						Value: proto.String("test-agent"),
					},
				},
				Counter: &io_prometheus_client.Counter{
					Value: proto.Float64(1),
				},
			},
		},
	}, protocmp.Transform()))

	// compare the baz metric
	require.Contains(t, metricsMap, "baz")
	assert.Empty(t, cmp.Diff(metricsMap["baz"], &io_prometheus_client.MetricFamily{
		Name: proto.String("baz"),
		Type: io_prometheus_client.MetricType_GAUGE.Enum(),
		Help: proto.String("bazhelp"),
		Metric: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{
					{
						Name:  proto.String(remoteAgentMetricTagName),
						Value: proto.String("test-agent"),
					},
					{
						Name:  proto.String("tag_one"),
						Value: proto.String("1"),
					},
					{
						Name:  proto.String("tag_two"),
						Value: proto.String("two"),
					},
				},
				Gauge: &io_prometheus_client.Gauge{
					Value: proto.Float64(3),
				},
			},
		},
	}, protocmp.Transform()))

	// compare the qux histogram metric
	require.Contains(t, metricsMap, "qux")
	assert.Empty(t, cmp.Diff(metricsMap["qux"], &io_prometheus_client.MetricFamily{
		Name: proto.String("qux"),
		Type: io_prometheus_client.MetricType_HISTOGRAM.Enum(),
		Help: proto.String("quxhelp"),
		Metric: []*io_prometheus_client.Metric{
			{
				Label: []*io_prometheus_client.LabelPair{
					{
						Name:  proto.String(remoteAgentMetricTagName),
						Value: proto.String("test-agent"),
					},
				},
				Histogram: &io_prometheus_client.Histogram{
					SampleCount: proto.Uint64(3),
					SampleSum:   proto.Float64(2.5),
					Bucket: []*io_prometheus_client.Bucket{
						{
							CumulativeCount: proto.Uint64(1),
							UpperBound:      proto.Float64(0.5),
						},
						{
							CumulativeCount: proto.Uint64(2),
							UpperBound:      proto.Float64(1),
						},
						{
							CumulativeCount: proto.Uint64(3),
							UpperBound:      proto.Float64(math.Inf(1)),
						},
					},
				},
			},
		},
	}, protocmp.Transform()))
}

// TestGetTelemetryPreservesExistingRemoteAgentLabel verifies that when a metric already
// has a remote_agent label (set by the remote agent itself via metrics.SetAgentIdentity),
// the registry collector preserves it and does NOT add a duplicate.
func TestGetTelemetryPreservesExistingRemoteAgentLabel(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	lc.Start(context.Background())
	component := provides.Comp

	// Simulate system-probe forwarding a metric that already has remote_agent="system-probe"
	// set via metrics.SetAgentIdentity("system-probe").
	promText := `
		# HELP logs__bytes_sent Total number of bytes sent
		# TYPE logs__bytes_sent counter
		logs__bytes_sent{remote_agent="system-probe",source="logs"} 42
		`

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "system-probe", "System Probe", "123",
		withTelemetryProvider(promText),
	)

	metrics, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	// Find the logs__bytes_sent metric and verify the remote_agent label
	require.Contains(t, metricsToMap(metrics), "logs__bytes_sent")

	for _, mf := range metrics {
		if mf.GetName() != "logs__bytes_sent" {
			continue
		}
		require.Len(t, mf.GetMetric(), 1)
		m := mf.GetMetric()[0]

		// Count remote_agent labels — there should be exactly one (not duplicated)
		remoteAgentCount := 0
		remoteAgentValue := ""
		for _, label := range m.GetLabel() {
			if label.GetName() == remoteAgentMetricTagName {
				remoteAgentCount++
				remoteAgentValue = label.GetValue()
			}
		}
		assert.Equal(t, 1, remoteAgentCount, "Should have exactly one remote_agent label, not a duplicate")
		assert.Equal(t, "system-probe", remoteAgentValue, "remote_agent value should be preserved from the metric, not overwritten by the registry")

		// Also verify the source label is preserved
		assert.Empty(t, cmp.Diff(mf, &io_prometheus_client.MetricFamily{
			Name: proto.String("logs__bytes_sent"),
			Type: io_prometheus_client.MetricType_COUNTER.Enum(),
			Help: proto.String("Total number of bytes sent"),
			Metric: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{
						{
							Name:  proto.String(remoteAgentMetricTagName),
							Value: proto.String("system-probe"),
						},
						{
							Name:  proto.String("source"),
							Value: proto.String("logs"),
						},
					},
					Counter: &io_prometheus_client.Counter{
						Value: proto.Float64(42),
					},
				},
			},
		}, protocmp.Transform()))
	}
}

// TestGetTelemetryMixedLabels verifies the registry handles a mix of metrics:
// some with pre-existing remote_agent labels and some without.
func TestGetTelemetryMixedLabels(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	lc.Start(context.Background())
	component := provides.Comp

	// Simulate an agent sending two metrics:
	// - logs__bytes_sent already has remote_agent (should be preserved)
	// - some_other_metric does NOT have remote_agent (should be injected by registry)
	promText := `
		# HELP logs__bytes_sent Total number of bytes sent
		# TYPE logs__bytes_sent counter
		logs__bytes_sent{remote_agent="system-probe",source="logs"} 100
		# HELP some_other_metric Another metric
		# TYPE some_other_metric gauge
		some_other_metric{tag="value"} 7
		`

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "system-probe", "System Probe", "456",
		withTelemetryProvider(promText),
	)

	metrics, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	metricsMap := metricsToMap(metrics)

	// logs__bytes_sent: remote_agent should be "system-probe" (from the metric itself)
	require.Contains(t, metricsMap, "logs__bytes_sent")
	for _, m := range metricsMap["logs__bytes_sent"].GetMetric() {
		for _, label := range m.GetLabel() {
			if label.GetName() == remoteAgentMetricTagName {
				assert.Equal(t, "system-probe", label.GetValue(), "Pre-existing remote_agent should be preserved")
			}
		}
	}

	// some_other_metric: remote_agent should be "system-probe" (injected by registry from display name)
	require.Contains(t, metricsMap, "some_other_metric")
	for _, m := range metricsMap["some_other_metric"].GetMetric() {
		foundRemoteAgent := false
		for _, label := range m.GetLabel() {
			if label.GetName() == remoteAgentMetricTagName {
				foundRemoteAgent = true
				assert.Equal(t, "system-probe", label.GetValue(), "Registry should inject remote_agent for metrics without it")
			}
		}
		assert.True(t, foundRemoteAgent, "Registry should add remote_agent label when missing")
	}
}

func metricsToMap(metrics []*io_prometheus_client.MetricFamily) map[string]*io_prometheus_client.MetricFamily {
	m := make(map[string]*io_prometheus_client.MetricFamily)
	for _, mf := range metrics {
		m[mf.GetName()] = mf
	}
	return m
}

// TestHistogramBucketMismatch tests that histogram metrics from remote agents with different
// bucket configurations do not cause panics or errors. This tests the reviewer concern about
// bucket mismatch conflicts.
//
// Key scenarios tested:
// 1. Remote agent sends histogram with same name as internal Agent metric but different buckets
// 2. Two remote agents send same histogram name with different bucket configurations
// 3. Same remote agent changes bucket configuration between collection cycles
func TestHistogramBucketMismatch(t *testing.T) {
	t.Run("scenario 1: histogram same name as internal metric with different buckets", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// This histogram has the same name as the internal RAR metric "remote_agent_registry_action_duration_seconds"
		// but with completely different bucket boundaries (1, 10, +Inf vs prometheus.DefBuckets)
		promText := `
# HELP remote_agent_registry_action_duration_seconds Conflicting histogram - same name as internal metric
# TYPE remote_agent_registry_action_duration_seconds histogram
remote_agent_registry_action_duration_seconds_bucket{le="1"} 5
remote_agent_registry_action_duration_seconds_bucket{le="10"} 15
remote_agent_registry_action_duration_seconds_bucket{le="+Inf"} 20
remote_agent_registry_action_duration_seconds_sum 50.0
remote_agent_registry_action_duration_seconds_count 20
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
			withTelemetryProvider(promText),
		)

		// This should NOT panic - the remote agent metric uses ConstHistogram which is not registered
		// and the remote_agent label provides namespace separation
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		// Find all metrics with the name "remote_agent_registry_action_duration_seconds"
		var foundRemoteAgentHistogram bool
		for _, mf := range metrics {
			if mf.GetName() == "remote_agent_registry_action_duration_seconds" {
				for _, m := range mf.GetMetric() {
					// Check if this metric has the remote_agent label (from the remote agent)
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName && label.GetValue() == "test-agent" {
							foundRemoteAgentHistogram = true
							// Verify the bucket configuration is from the remote agent (3 buckets)
							require.NotNil(t, m.GetHistogram())
							require.Len(t, m.GetHistogram().GetBucket(), 3, "Expected 3 buckets from remote agent")
							require.Equal(t, uint64(20), m.GetHistogram().GetSampleCount())
						}
					}
				}
			}
		}
		require.True(t, foundRemoteAgentHistogram, "Remote agent histogram should be present")
	})

	t.Run("scenario 2: two remote agents with same histogram name but different buckets", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Agent 1: histogram with buckets [0.1, 1.0, +Inf]
		promText1 := `
# HELP my_shared_histogram Shared histogram
# TYPE my_shared_histogram histogram
my_shared_histogram_bucket{le="0.1"} 10
my_shared_histogram_bucket{le="1.0"} 20
my_shared_histogram_bucket{le="+Inf"} 25
my_shared_histogram_sum 10.0
my_shared_histogram_count 25
`

		// Agent 2: histogram with completely different buckets [5, 50, 500, +Inf]
		promText2 := `
# HELP my_shared_histogram Shared histogram
# TYPE my_shared_histogram histogram
my_shared_histogram_bucket{le="5"} 100
my_shared_histogram_bucket{le="50"} 200
my_shared_histogram_bucket{le="500"} 250
my_shared_histogram_bucket{le="+Inf"} 300
my_shared_histogram_sum 1000.0
my_shared_histogram_count 300
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-one", "Agent One", "111",
			withTelemetryProvider(promText1),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-two", "Agent Two", "222",
			withTelemetryProvider(promText2),
		)

		// This should NOT panic - both agents should coexist with their different bucket configs
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var foundAgent1, foundAgent2 bool
		for _, mf := range metrics {
			if mf.GetName() == "my_shared_histogram" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName {
							switch label.GetValue() {
							case "agent-one":
								foundAgent1 = true
								require.Len(t, m.GetHistogram().GetBucket(), 3, "Agent 1 should have 3 buckets")
								require.Equal(t, uint64(25), m.GetHistogram().GetSampleCount())
							case "agent-two":
								foundAgent2 = true
								require.Len(t, m.GetHistogram().GetBucket(), 4, "Agent 2 should have 4 buckets")
								require.Equal(t, uint64(300), m.GetHistogram().GetSampleCount())
							}
						}
					}
				}
			}
		}
		require.True(t, foundAgent1, "Agent 1 histogram should be present")
		require.True(t, foundAgent2, "Agent 2 histogram should be present")
	})

	t.Run("scenario 3: histogram with labels matching internal metric labels", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// This histogram tries to use the same labels ("name", "action") as the internal metric
		// but the remote_agent label will still be injected, making them distinct
		promText := `
# HELP remote_agent_registry_action_duration_seconds Histogram trying to match internal labels
# TYPE remote_agent_registry_action_duration_seconds histogram
remote_agent_registry_action_duration_seconds_bucket{name="fake-agent",action="query",le="0.5"} 10
remote_agent_registry_action_duration_seconds_bucket{name="fake-agent",action="query",le="2.0"} 25
remote_agent_registry_action_duration_seconds_bucket{name="fake-agent",action="query",le="+Inf"} 30
remote_agent_registry_action_duration_seconds_sum{name="fake-agent",action="query"} 12.5
remote_agent_registry_action_duration_seconds_count{name="fake-agent",action="query"} 30
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "sneaky-agent", "Sneaky Agent", "999",
			withTelemetryProvider(promText),
		)

		// This should NOT panic - the remote_agent label is always injected
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var foundSneakyHistogram bool
		for _, mf := range metrics {
			if mf.GetName() == "remote_agent_registry_action_duration_seconds" {
				for _, m := range mf.GetMetric() {
					labels := m.GetLabel()
					// Check for remote_agent label being present and find it among all labels
					var hasRemoteAgentLabel, hasNameLabel, hasActionLabel bool
					for _, label := range labels {
						switch label.GetName() {
						case remoteAgentMetricTagName:
							if label.GetValue() == "sneaky-agent" {
								hasRemoteAgentLabel = true
							}
						case "name":
							if label.GetValue() == "fake-agent" {
								hasNameLabel = true
							}
						case "action":
							if label.GetValue() == "query" {
								hasActionLabel = true
							}
						}
					}
					if hasRemoteAgentLabel && hasNameLabel && hasActionLabel {
						foundSneakyHistogram = true
						// Verify all 3 labels are present: remote_agent, name, action
						require.Len(t, labels, 3, "Should have exactly 3 labels")
					}
				}
			}
		}
		require.True(t, foundSneakyHistogram, "Sneaky agent histogram should be present with remote_agent label")
	})

	t.Run("scenario 4: unique histogram name works fine", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		promText := `
# HELP completely_unique_histogram A histogram with a unique name
# TYPE completely_unique_histogram histogram
completely_unique_histogram_bucket{le="0.1"} 5
completely_unique_histogram_bucket{le="0.5"} 15
completely_unique_histogram_bucket{le="1.0"} 25
completely_unique_histogram_bucket{le="5.0"} 35
completely_unique_histogram_bucket{le="+Inf"} 40
completely_unique_histogram_sum 20.5
completely_unique_histogram_count 40
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "unique-agent", "Unique Agent", "456",
			withTelemetryProvider(promText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var foundUniqueHistogram bool
		for _, mf := range metrics {
			if mf.GetName() == "completely_unique_histogram" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName && label.GetValue() == "unique-agent" {
							foundUniqueHistogram = true
							require.NotNil(t, m.GetHistogram())
							require.Len(t, m.GetHistogram().GetBucket(), 5, "Expected 5 buckets")
							require.Equal(t, uint64(40), m.GetHistogram().GetSampleCount())
							require.Equal(t, float64(20.5), m.GetHistogram().GetSampleSum())
						}
					}
				}
			}
		}
		require.True(t, foundUniqueHistogram, "Unique histogram should be present")
	})
}

// TestDescriptorConflicts tests scenarios where metric descriptors might conflict
// This is a potential source of panics identified during code review
func TestDescriptorConflicts(t *testing.T) {
	t.Run("two agents same histogram exact same labels", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Both agents send the EXACT same metric with same labels - only remote_agent will differ
		promText := `
# HELP shared_request_duration Request duration
# TYPE shared_request_duration histogram
shared_request_duration_bucket{method="GET",path="/api",le="0.1"} 10
shared_request_duration_bucket{method="GET",path="/api",le="0.5"} 25
shared_request_duration_bucket{method="GET",path="/api",le="+Inf"} 30
shared_request_duration_sum{method="GET",path="/api"} 15.0
shared_request_duration_count{method="GET",path="/api"} 30
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-a", "Agent A", "111",
			withTelemetryProvider(promText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-b", "Agent B", "222",
			withTelemetryProvider(promText),
		)

		// This tests if having identical metrics (except remote_agent label) causes issues
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var agentAFound, agentBFound bool
		for _, mf := range metrics {
			if mf.GetName() == "shared_request_duration" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName {
							switch label.GetValue() {
							case "agent-a":
								agentAFound = true
							case "agent-b":
								agentBFound = true
							}
						}
					}
				}
			}
		}
		require.True(t, agentAFound, "Agent A metrics should be present")
		require.True(t, agentBFound, "Agent B metrics should be present")
	})

	t.Run("same metric name different help strings causes error", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Two agents send metrics with same name but DIFFERENT help strings
		// This is a KNOWN ISSUE - Prometheus validation will fail
		agent1PromText := `
# HELP requests Total requests (agent 1)
# TYPE requests counter
requests 100
`
		agent2PromText := `
# HELP requests Total requests (agent 2 different help!)
# TYPE requests counter
requests 200
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-help-1", "Agent Help 1", "111",
			withTelemetryProvider(agent1PromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-help-2", "Agent Help 2", "222",
			withTelemetryProvider(agent2PromText),
		)

		// THIS WILL FAIL - Prometheus doesn't allow same metric name with different help strings
		// This is a real limitation/bug that should be documented
		_, err := telemetryComp.Gather(false)
		require.Error(t, err, "Expected error due to mismatched help strings")
		require.Contains(t, err.Error(), "has help", "Error should mention help string mismatch")
	})

	t.Run("same metric name same help different types causes error", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Two agents send metrics with same name and help but DIFFERENT types
		counterPromText := `
# HELP requests Total requests
# TYPE requests counter
requests 100
`
		gaugePromText := `
# HELP requests Total requests
# TYPE requests gauge
requests 50
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "counter-agent", "Counter Agent", "111",
			withTelemetryProvider(counterPromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "gauge-agent", "Gauge Agent", "222",
			withTelemetryProvider(gaugePromText),
		)

		// THIS WILL ALSO FAIL - Prometheus doesn't allow same metric name with different types
		_, err := telemetryComp.Gather(false)
		require.Error(t, err, "Expected error due to mismatched metric types")
	})

	t.Run("same metric name different label sets between agents", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Agent 1: metric with labels {method, path}
		agent1PromText := `
# HELP http_requests HTTP requests
# TYPE http_requests counter
http_requests{method="GET",path="/api"} 100
`
		// Agent 2: same metric name but different labels {endpoint, status}
		agent2PromText := `
# HELP http_requests HTTP requests
# TYPE http_requests counter
http_requests{endpoint="api",status="200"} 50
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-labels-1", "Agent Labels 1", "111",
			withTelemetryProvider(agent1PromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-labels-2", "Agent Labels 2", "222",
			withTelemetryProvider(agent2PromText),
		)

		// This is the interesting case - same metric name but DIFFERENT label schemas
		// The remote_agent label is added to both, making them:
		// - http_requests{remote_agent="agent-labels-1", method="GET", path="/api"}
		// - http_requests{remote_agent="agent-labels-2", endpoint="api", status="200"}
		// These have different label sets which could cause issues with some Prometheus registries
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var agent1Found, agent2Found bool
		for _, mf := range metrics {
			if mf.GetName() == "http_requests" {
				for _, m := range mf.GetMetric() {
					labelNames := make(map[string]bool)
					for _, label := range m.GetLabel() {
						labelNames[label.GetName()] = true
						if label.GetName() == remoteAgentMetricTagName {
							switch label.GetValue() {
							case "agent-labels-1":
								agent1Found = true
								require.True(t, labelNames["method"] || labelContains(m.GetLabel(), "method"))
							case "agent-labels-2":
								agent2Found = true
								require.True(t, labelNames["endpoint"] || labelContains(m.GetLabel(), "endpoint"))
							}
						}
					}
				}
			}
		}
		require.True(t, agent1Found, "Agent 1 metric should be present")
		require.True(t, agent2Found, "Agent 2 metric should be present")
	})
}

func labelContains(labels []*io_prometheus_client.LabelPair, name string) bool {
	for _, l := range labels {
		if l.GetName() == name {
			return true
		}
	}
	return false
}

// TestHistogramBucketCountChanges specifically tests scenarios where the number of buckets
// changes between scrapes or differs between agents. This addresses reviewer feedback about
// errors/panics when "histograms didn't share the same amount of buckets".
func TestHistogramBucketCountChanges(t *testing.T) {
	t.Run("two agents same metric name different bucket counts", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Agent 1: 3 buckets
		agent1PromText := `
# HELP request_duration Request duration
# TYPE request_duration histogram
request_duration_bucket{le="0.1"} 10
request_duration_bucket{le="1"} 20
request_duration_bucket{le="+Inf"} 30
request_duration_sum 15.0
request_duration_count 30
`
		// Agent 2: 5 buckets (MORE buckets)
		agent2PromText := `
# HELP request_duration Request duration
# TYPE request_duration histogram
request_duration_bucket{le="0.05"} 5
request_duration_bucket{le="0.1"} 10
request_duration_bucket{le="0.5"} 15
request_duration_bucket{le="1"} 20
request_duration_bucket{le="+Inf"} 25
request_duration_sum 10.0
request_duration_count 25
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-3-buckets", "Agent 3 Buckets", "111",
			withTelemetryProvider(agent1PromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-5-buckets", "Agent 5 Buckets", "222",
			withTelemetryProvider(agent2PromText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Different bucket counts between agents should not cause error")

		var found3Buckets, found5Buckets bool
		for _, mf := range metrics {
			if mf.GetName() == "request_duration" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName {
							bucketCount := len(m.GetHistogram().GetBucket())
							switch label.GetValue() {
							case "agent-3-buckets":
								found3Buckets = true
								require.Equal(t, 3, bucketCount, "Agent 1 should have 3 buckets")
							case "agent-5-buckets":
								found5Buckets = true
								require.Equal(t, 5, bucketCount, "Agent 2 should have 5 buckets")
							}
						}
					}
				}
			}
		}
		require.True(t, found3Buckets, "3-bucket histogram should be present")
		require.True(t, found5Buckets, "5-bucket histogram should be present")
	})

	t.Run("single agent histogram with 1 bucket only", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Minimal histogram - just +Inf bucket
		promText := `
# HELP minimal_histogram Minimal histogram
# TYPE minimal_histogram histogram
minimal_histogram_bucket{le="+Inf"} 100
minimal_histogram_sum 500.0
minimal_histogram_count 100
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "minimal-agent", "Minimal Agent", "123",
			withTelemetryProvider(promText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Minimal histogram with 1 bucket should work")

		var found bool
		for _, mf := range metrics {
			if mf.GetName() == "minimal_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						found = true
						require.Equal(t, 1, len(m.GetHistogram().GetBucket()), "Should have 1 bucket")
					}
				}
			}
		}
		require.True(t, found, "Minimal histogram should be present")
	})

	t.Run("histogram with zero buckets", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Histogram with no bucket lines at all - just sum and count
		promText := `
# HELP no_bucket_histogram Histogram without buckets
# TYPE no_bucket_histogram histogram
no_bucket_histogram_sum 100.0
no_bucket_histogram_count 50
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "no-bucket-agent", "No Bucket Agent", "123",
			withTelemetryProvider(promText),
		)

		// This might fail or produce an empty histogram
		metrics, err := telemetryComp.Gather(false)
		// Document the actual behavior - don't assert success or failure yet
		t.Logf("Gather error for zero buckets: %v", err)
		t.Logf("Metrics count: %d", len(metrics))
	})

	t.Run("two agents same name same help but vastly different bucket counts", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Agent 1: 2 buckets (minimal)
		agent1PromText := `
# HELP api_latency API latency
# TYPE api_latency histogram
api_latency_bucket{le="1"} 10
api_latency_bucket{le="+Inf"} 15
api_latency_sum 5.0
api_latency_count 15
`
		// Agent 2: 20 buckets (many)
		agent2PromText := `
# HELP api_latency API latency
# TYPE api_latency histogram
api_latency_bucket{le="0.001"} 1
api_latency_bucket{le="0.002"} 2
api_latency_bucket{le="0.005"} 5
api_latency_bucket{le="0.01"} 10
api_latency_bucket{le="0.02"} 15
api_latency_bucket{le="0.05"} 25
api_latency_bucket{le="0.1"} 40
api_latency_bucket{le="0.2"} 55
api_latency_bucket{le="0.5"} 70
api_latency_bucket{le="1"} 80
api_latency_bucket{le="2"} 85
api_latency_bucket{le="5"} 90
api_latency_bucket{le="10"} 93
api_latency_bucket{le="20"} 95
api_latency_bucket{le="50"} 97
api_latency_bucket{le="100"} 98
api_latency_bucket{le="200"} 99
api_latency_bucket{le="500"} 99
api_latency_bucket{le="1000"} 100
api_latency_bucket{le="+Inf"} 100
api_latency_sum 250.0
api_latency_count 100
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-2-buckets", "Agent 2 Buckets", "111",
			withTelemetryProvider(agent1PromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-20-buckets", "Agent 20 Buckets", "222",
			withTelemetryProvider(agent2PromText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Vastly different bucket counts should not cause error")

		var found2, found20 bool
		for _, mf := range metrics {
			if mf.GetName() == "api_latency" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName {
							bucketCount := len(m.GetHistogram().GetBucket())
							t.Logf("Agent %s has %d buckets", label.GetValue(), bucketCount)
							switch label.GetValue() {
							case "agent-2-buckets":
								found2 = true
								require.Equal(t, 2, bucketCount)
							case "agent-20-buckets":
								found20 = true
								require.Equal(t, 20, bucketCount)
							}
						}
					}
				}
			}
		}
		require.True(t, found2, "2-bucket histogram should be present")
		require.True(t, found20, "20-bucket histogram should be present")
	})

	t.Run("histogram bucket boundaries subset vs superset", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Agent 1: buckets at [0.1, 0.5, 1, +Inf]
		agent1PromText := `
# HELP response_time Response time
# TYPE response_time histogram
response_time_bucket{le="0.1"} 10
response_time_bucket{le="0.5"} 30
response_time_bucket{le="1"} 45
response_time_bucket{le="+Inf"} 50
response_time_sum 25.0
response_time_count 50
`
		// Agent 2: buckets at [0.5, 1, +Inf] - SUBSET of agent 1's boundaries
		agent2PromText := `
# HELP response_time Response time
# TYPE response_time histogram
response_time_bucket{le="0.5"} 20
response_time_bucket{le="1"} 35
response_time_bucket{le="+Inf"} 40
response_time_sum 20.0
response_time_count 40
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-superset", "Agent Superset", "111",
			withTelemetryProvider(agent1PromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-subset", "Agent Subset", "222",
			withTelemetryProvider(agent2PromText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Subset/superset bucket boundaries should not cause error")

		var foundSuperset, foundSubset bool
		for _, mf := range metrics {
			if mf.GetName() == "response_time" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName {
							switch label.GetValue() {
							case "agent-superset":
								foundSuperset = true
								require.Equal(t, 4, len(m.GetHistogram().GetBucket()))
							case "agent-subset":
								foundSubset = true
								require.Equal(t, 3, len(m.GetHistogram().GetBucket()))
							}
						}
					}
				}
			}
		}
		require.True(t, foundSuperset, "Superset histogram should be present")
		require.True(t, foundSubset, "Subset histogram should be present")
	})

	t.Run("histogram with overlapping but different bucket boundaries", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Agent 1: buckets at [0.1, 0.25, 0.5, 1, +Inf]
		agent1PromText := `
# HELP process_time Process time
# TYPE process_time histogram
process_time_bucket{le="0.1"} 10
process_time_bucket{le="0.25"} 20
process_time_bucket{le="0.5"} 30
process_time_bucket{le="1"} 40
process_time_bucket{le="+Inf"} 50
process_time_sum 25.0
process_time_count 50
`
		// Agent 2: buckets at [0.1, 0.3, 0.5, 1, +Inf] - same count but 0.3 instead of 0.25
		agent2PromText := `
# HELP process_time Process time
# TYPE process_time histogram
process_time_bucket{le="0.1"} 10
process_time_bucket{le="0.3"} 22
process_time_bucket{le="0.5"} 30
process_time_bucket{le="1"} 40
process_time_bucket{le="+Inf"} 50
process_time_sum 25.0
process_time_count 50
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-boundaries-1", "Agent Boundaries 1", "111",
			withTelemetryProvider(agent1PromText),
		)
		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "agent-boundaries-2", "Agent Boundaries 2", "222",
			withTelemetryProvider(agent2PromText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Different bucket boundaries with same count should not cause error")

		var found1, found2 bool
		for _, mf := range metrics {
			if mf.GetName() == "process_time" {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == remoteAgentMetricTagName {
							switch label.GetValue() {
							case "agent-boundaries-1":
								found1 = true
								// Verify it has the 0.25 bucket
								buckets := m.GetHistogram().GetBucket()
								var has025 bool
								for _, b := range buckets {
									if b.GetUpperBound() == 0.25 {
										has025 = true
									}
								}
								require.True(t, has025, "Agent 1 should have 0.25 bucket")
							case "agent-boundaries-2":
								found2 = true
								// Verify it has the 0.3 bucket
								buckets := m.GetHistogram().GetBucket()
								var has03 bool
								for _, b := range buckets {
									if b.GetUpperBound() == 0.3 {
										has03 = true
									}
								}
								require.True(t, has03, "Agent 2 should have 0.3 bucket")
							}
						}
					}
				}
			}
		}
		require.True(t, found1, "Agent 1 histogram should be present")
		require.True(t, found2, "Agent 2 histogram should be present")
	})
}

// TestHistogramBucketChangeBetweenScrapes tests scenarios where an agent's histogram
// bucket configuration changes between consecutive scrapes. This is a key scenario
// that could trigger issues with Prometheus registry consistency.
func TestHistogramBucketChangeBetweenScrapes(t *testing.T) {
	t.Run("bucket count increases between scrapes", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// First scrape: 3 buckets
		initialPromText := `
# HELP changing_histogram Histogram that changes
# TYPE changing_histogram histogram
changing_histogram_bucket{le="0.1"} 10
changing_histogram_bucket{le="1"} 20
changing_histogram_bucket{le="+Inf"} 30
changing_histogram_sum 15.0
changing_histogram_count 30
`

		remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "changing-agent", "Changing Agent", "123",
			withTelemetryProvider(initialPromText),
		)

		// First scrape
		metrics1, err := telemetryComp.Gather(false)
		require.NoError(t, err, "First scrape should succeed")

		var firstBucketCount int
		for _, mf := range metrics1 {
			if mf.GetName() == "changing_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						firstBucketCount = len(m.GetHistogram().GetBucket())
					}
				}
			}
		}
		require.Equal(t, 3, firstBucketCount, "First scrape should have 3 buckets")

		// Change the histogram to have MORE buckets (5 buckets)
		remoteAgent.promText = `
# HELP changing_histogram Histogram that changes
# TYPE changing_histogram histogram
changing_histogram_bucket{le="0.05"} 5
changing_histogram_bucket{le="0.1"} 15
changing_histogram_bucket{le="0.5"} 25
changing_histogram_bucket{le="1"} 35
changing_histogram_bucket{le="+Inf"} 45
changing_histogram_sum 25.0
changing_histogram_count 45
`

		// Second scrape - bucket count increased
		metrics2, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Second scrape with more buckets should succeed")

		var secondBucketCount int
		for _, mf := range metrics2 {
			if mf.GetName() == "changing_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						secondBucketCount = len(m.GetHistogram().GetBucket())
					}
				}
			}
		}
		require.Equal(t, 5, secondBucketCount, "Second scrape should have 5 buckets")
	})

	t.Run("bucket count decreases between scrapes", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// First scrape: 5 buckets
		initialPromText := `
# HELP shrinking_histogram Histogram that shrinks
# TYPE shrinking_histogram histogram
shrinking_histogram_bucket{le="0.05"} 5
shrinking_histogram_bucket{le="0.1"} 15
shrinking_histogram_bucket{le="0.5"} 25
shrinking_histogram_bucket{le="1"} 35
shrinking_histogram_bucket{le="+Inf"} 45
shrinking_histogram_sum 25.0
shrinking_histogram_count 45
`

		remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "shrinking-agent", "Shrinking Agent", "123",
			withTelemetryProvider(initialPromText),
		)

		// First scrape
		metrics1, err := telemetryComp.Gather(false)
		require.NoError(t, err, "First scrape should succeed")

		var firstBucketCount int
		for _, mf := range metrics1 {
			if mf.GetName() == "shrinking_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						firstBucketCount = len(m.GetHistogram().GetBucket())
					}
				}
			}
		}
		require.Equal(t, 5, firstBucketCount, "First scrape should have 5 buckets")

		// Change the histogram to have FEWER buckets (2 buckets)
		remoteAgent.promText = `
# HELP shrinking_histogram Histogram that shrinks
# TYPE shrinking_histogram histogram
shrinking_histogram_bucket{le="1"} 30
shrinking_histogram_bucket{le="+Inf"} 40
shrinking_histogram_sum 20.0
shrinking_histogram_count 40
`

		// Second scrape - bucket count decreased
		metrics2, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Second scrape with fewer buckets should succeed")

		var secondBucketCount int
		for _, mf := range metrics2 {
			if mf.GetName() == "shrinking_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						secondBucketCount = len(m.GetHistogram().GetBucket())
					}
				}
			}
		}
		require.Equal(t, 2, secondBucketCount, "Second scrape should have 2 buckets")
	})

	t.Run("bucket boundaries change between scrapes same count", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// First scrape: buckets at [0.1, 0.5, 1, +Inf]
		initialPromText := `
# HELP boundary_histogram Histogram with changing boundaries
# TYPE boundary_histogram histogram
boundary_histogram_bucket{le="0.1"} 10
boundary_histogram_bucket{le="0.5"} 30
boundary_histogram_bucket{le="1"} 45
boundary_histogram_bucket{le="+Inf"} 50
boundary_histogram_sum 25.0
boundary_histogram_count 50
`

		remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "boundary-agent", "Boundary Agent", "123",
			withTelemetryProvider(initialPromText),
		)

		// First scrape
		metrics1, err := telemetryComp.Gather(false)
		require.NoError(t, err, "First scrape should succeed")

		var firstBoundaries []float64
		for _, mf := range metrics1 {
			if mf.GetName() == "boundary_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						for _, b := range m.GetHistogram().GetBucket() {
							firstBoundaries = append(firstBoundaries, b.GetUpperBound())
						}
					}
				}
			}
		}
		require.Len(t, firstBoundaries, 4, "First scrape should have 4 buckets")

		// Change bucket boundaries (same count, different boundaries)
		// [0.1, 0.5, 1, +Inf] -> [0.2, 0.6, 2, +Inf]
		remoteAgent.promText = `
# HELP boundary_histogram Histogram with changing boundaries
# TYPE boundary_histogram histogram
boundary_histogram_bucket{le="0.2"} 15
boundary_histogram_bucket{le="0.6"} 35
boundary_histogram_bucket{le="2"} 48
boundary_histogram_bucket{le="+Inf"} 55
boundary_histogram_sum 30.0
boundary_histogram_count 55
`

		// Second scrape - same bucket count but different boundaries
		metrics2, err := telemetryComp.Gather(false)
		require.NoError(t, err, "Second scrape with different boundaries should succeed")

		var secondBoundaries []float64
		for _, mf := range metrics2 {
			if mf.GetName() == "boundary_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						for _, b := range m.GetHistogram().GetBucket() {
							secondBoundaries = append(secondBoundaries, b.GetUpperBound())
						}
					}
				}
			}
		}
		require.Len(t, secondBoundaries, 4, "Second scrape should have 4 buckets")
		// Verify the boundaries actually changed
		require.NotEqual(t, firstBoundaries, secondBoundaries, "Boundaries should have changed")
	})

	t.Run("multiple rapid bucket changes", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		initialPromText := `
# HELP rapid_histogram Histogram with rapid changes
# TYPE rapid_histogram histogram
rapid_histogram_bucket{le="1"} 10
rapid_histogram_bucket{le="+Inf"} 20
rapid_histogram_sum 10.0
rapid_histogram_count 20
`

		remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "rapid-agent", "Rapid Agent", "123",
			withTelemetryProvider(initialPromText),
		)

		// Perform multiple scrapes with different bucket configurations
		bucketConfigs := []struct {
			promText      string
			expectedCount int
		}{
			{
				promText: `
# HELP rapid_histogram Histogram with rapid changes
# TYPE rapid_histogram histogram
rapid_histogram_bucket{le="1"} 10
rapid_histogram_bucket{le="+Inf"} 20
rapid_histogram_sum 10.0
rapid_histogram_count 20
`,
				expectedCount: 2,
			},
			{
				promText: `
# HELP rapid_histogram Histogram with rapid changes
# TYPE rapid_histogram histogram
rapid_histogram_bucket{le="0.5"} 5
rapid_histogram_bucket{le="1"} 15
rapid_histogram_bucket{le="5"} 25
rapid_histogram_bucket{le="+Inf"} 30
rapid_histogram_sum 15.0
rapid_histogram_count 30
`,
				expectedCount: 4,
			},
			{
				promText: `
# HELP rapid_histogram Histogram with rapid changes
# TYPE rapid_histogram histogram
rapid_histogram_bucket{le="10"} 50
rapid_histogram_bucket{le="+Inf"} 60
rapid_histogram_sum 30.0
rapid_histogram_count 60
`,
				expectedCount: 2,
			},
			{
				promText: `
# HELP rapid_histogram Histogram with rapid changes
# TYPE rapid_histogram histogram
rapid_histogram_bucket{le="0.01"} 1
rapid_histogram_bucket{le="0.1"} 10
rapid_histogram_bucket{le="1"} 50
rapid_histogram_bucket{le="10"} 80
rapid_histogram_bucket{le="100"} 95
rapid_histogram_bucket{le="+Inf"} 100
rapid_histogram_sum 50.0
rapid_histogram_count 100
`,
				expectedCount: 6,
			},
		}

		for i, config := range bucketConfigs {
			remoteAgent.promText = config.promText

			metrics, err := telemetryComp.Gather(false)
			require.NoError(t, err, "Scrape %d should succeed", i+1)

			var bucketCount int
			for _, mf := range metrics {
				if mf.GetName() == "rapid_histogram" {
					for _, m := range mf.GetMetric() {
						if m.GetHistogram() != nil {
							bucketCount = len(m.GetHistogram().GetBucket())
						}
					}
				}
			}
			require.Equal(t, config.expectedCount, bucketCount, "Scrape %d should have %d buckets", i+1, config.expectedCount)
		}
	})
}

// TestMalformedMetricEdgeCases tests edge cases with malformed or unusual metric data
// that could potentially cause panics or errors
func TestMalformedMetricEdgeCases(t *testing.T) {
	t.Run("empty prometheus text", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Empty telemetry should not panic
		promText := ``

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "empty-agent", "Empty Agent", "123",
			withTelemetryProvider(promText),
		)

		// Should not panic
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)
		// No metrics from the empty agent, but gather should succeed
		_ = metrics
	})

	t.Run("whitespace only prometheus text", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		promText := `

		`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "whitespace-agent", "Whitespace Agent", "123",
			withTelemetryProvider(promText),
		)

		// Should not panic
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)
		_ = metrics
	})

	t.Run("histogram with missing +Inf bucket", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Histogram without the required +Inf bucket - parser may handle this differently
		promText := `
# HELP no_inf_histogram Histogram without +Inf
# TYPE no_inf_histogram histogram
no_inf_histogram_bucket{le="1"} 10
no_inf_histogram_bucket{le="5"} 20
no_inf_histogram_sum 15.0
no_inf_histogram_count 20
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "no-inf-agent", "No Inf Agent", "123",
			withTelemetryProvider(promText),
		)

		// Should not panic - the parser should handle this gracefully
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)
		_ = metrics
	})

	t.Run("histogram with non-cumulative bucket counts", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Non-cumulative bucket counts (invalid but should not panic)
		promText := `
# HELP bad_histogram Histogram with bad bucket counts
# TYPE bad_histogram histogram
bad_histogram_bucket{le="1"} 50
bad_histogram_bucket{le="5"} 30
bad_histogram_bucket{le="+Inf"} 100
bad_histogram_sum 100.0
bad_histogram_count 100
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "bad-bucket-agent", "Bad Bucket Agent", "123",
			withTelemetryProvider(promText),
		)

		// Should not panic - the implementation accepts whatever buckets are passed
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)
		_ = metrics
	})

	t.Run("metric with empty label value", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Metric with empty label value
		promText := `
# HELP empty_label_metric Metric with empty label value
# TYPE empty_label_metric gauge
empty_label_metric{tag=""} 42
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "empty-label-agent", "Empty Label Agent", "123",
			withTelemetryProvider(promText),
		)

		// Should not panic
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)
		_ = metrics
	})

	t.Run("metric with special characters in label", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Metric with special characters that might cause issues
		promText := `
# HELP special_char_metric Metric with special chars
# TYPE special_char_metric gauge
special_char_metric{path="/api/v1/test",method="GET"} 100
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "special-char-agent", "Special Char Agent", "123",
			withTelemetryProvider(promText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var foundMetric bool
		for _, mf := range metrics {
			if mf.GetName() == "special_char_metric" {
				foundMetric = true
			}
		}
		require.True(t, foundMetric, "Metric with special characters should be present")
	})

	t.Run("very large histogram bucket count", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Histogram with many buckets
		promText := `
# HELP many_buckets_histogram Histogram with many buckets
# TYPE many_buckets_histogram histogram
many_buckets_histogram_bucket{le="0.001"} 1
many_buckets_histogram_bucket{le="0.005"} 5
many_buckets_histogram_bucket{le="0.01"} 10
many_buckets_histogram_bucket{le="0.025"} 20
many_buckets_histogram_bucket{le="0.05"} 30
many_buckets_histogram_bucket{le="0.075"} 40
many_buckets_histogram_bucket{le="0.1"} 50
many_buckets_histogram_bucket{le="0.25"} 100
many_buckets_histogram_bucket{le="0.5"} 150
many_buckets_histogram_bucket{le="0.75"} 175
many_buckets_histogram_bucket{le="1"} 200
many_buckets_histogram_bucket{le="2.5"} 220
many_buckets_histogram_bucket{le="5"} 230
many_buckets_histogram_bucket{le="7.5"} 235
many_buckets_histogram_bucket{le="10"} 240
many_buckets_histogram_bucket{le="+Inf"} 250
many_buckets_histogram_sum 500.0
many_buckets_histogram_count 250
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "many-bucket-agent", "Many Bucket Agent", "123",
			withTelemetryProvider(promText),
		)

		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var foundHistogram bool
		for _, mf := range metrics {
			if mf.GetName() == "many_buckets_histogram" {
				for _, m := range mf.GetMetric() {
					if m.GetHistogram() != nil {
						foundHistogram = true
						require.Len(t, m.GetHistogram().GetBucket(), 16, "Should have 16 buckets")
					}
				}
			}
		}
		require.True(t, foundHistogram, "Histogram with many buckets should be present")
	})

	t.Run("duplicate metric names from same agent", func(t *testing.T) {
		provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
		lc.Start(context.Background())
		defer lc.Stop(context.Background())
		component := provides.Comp

		// Same metric name declared twice - parser behavior varies
		promText := `
# HELP duplicate_metric A metric
# TYPE duplicate_metric counter
duplicate_metric{instance="a"} 10
duplicate_metric{instance="b"} 20
`

		_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "dup-agent", "Dup Agent", "123",
			withTelemetryProvider(promText),
		)

		// Should not panic - this is actually valid (same metric with different label values)
		metrics, err := telemetryComp.Gather(false)
		require.NoError(t, err)

		var metricCount int
		for _, mf := range metrics {
			if mf.GetName() == "duplicate_metric" {
				metricCount = len(mf.GetMetric())
			}
		}
		require.Equal(t, 2, metricCount, "Should have 2 metric instances")
	})
}

func TestStatusProvider(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp
	statusProvider := provides.Status

	_ = buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "1234",
		withStatusProvider(map[string]string{
			"test_key": "test_value",
		}, nil),
	)

	statusData := make(map[string]interface{})
	err := statusProvider.Provider.JSON(false, statusData)
	require.NoError(t, err)

	require.Len(t, statusData, 2)

	registeredAgents, ok := statusData["registeredAgents"].([]remoteagent.RegisteredAgent)
	if !ok {
		t.Fatalf("registeredAgents is not a slice of RegisteredAgent")
	}
	require.Len(t, registeredAgents, 1)
	require.Equal(t, "Test Agent", registeredAgents[0].DisplayName)

	registeredAgentStatuses, ok := statusData["registeredAgentStatuses"].([]remoteagent.StatusData)
	if !ok {
		t.Fatalf("registeredAgentStatuses is not a slice of StatusData")
	}
	require.Len(t, registeredAgentStatuses, 1)
	require.Equal(t, "test-agent", registeredAgentStatuses[0].Flavor)
	require.Equal(t, "1234", registeredAgentStatuses[0].PID)
	require.Equal(t, "Test Agent", registeredAgentStatuses[0].DisplayName)
	require.Equal(t, "test_value", registeredAgentStatuses[0].MainSection["test_key"])
}
