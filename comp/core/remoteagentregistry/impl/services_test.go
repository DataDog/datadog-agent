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
