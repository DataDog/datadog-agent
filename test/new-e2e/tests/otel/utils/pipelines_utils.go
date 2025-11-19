// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains util functions for OTel e2e tests
package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	// CalendarService is the default service value for the calendar app
	CalendarService          = "calendar-rest-go"
	calendarTopLevelResource = "GET"

	env                  = "e2e"
	version              = "1.0"
	customAttribute      = "custom.attribute"
	customAttributeValue = "true"
	log1Body             = "getting random date"
	log2Body             = "random date"
)

// OTelTestSuite is an interface for the OTel e2e test suite.
type OTelTestSuite interface {
	T() *testing.T
	Env() *environments.Kubernetes
}

// IAParams contains options for different infra attribute testing scenarios
type IAParams struct {
	// InfraAttributes indicates whether this test should check for infra attributes
	InfraAttributes bool

	// EKS indicates if this test should check for EKS specific properties
	EKS bool

	// Cardinality represents the tag cardinality used by this test
	Cardinality types.TagCardinality
}

// TestTraces tests that OTLP traces are received through OTel pipelines as expected
func TestTraces(s OTelTestSuite, iaParams IAParams) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var traces []*aggregator.TracePayload
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotEmpty(c, traces) {
			return
		}
		trace := traces[0]
		if !assert.NotEmpty(s.T(), trace.TracerPayloads) {
			return
		}
		tp := trace.TracerPayloads[0]
		if !assert.NotEmpty(s.T(), tp.Chunks) {
			return
		}
		if !assert.NotEmpty(s.T(), tp.Chunks[0].Spans) {
			return
		}
		assert.Equal(s.T(), CalendarService, tp.Chunks[0].Spans[0].Service)
		if iaParams.InfraAttributes {
			ctags, ok := getContainerTags(s.T(), tp)
			assert.True(s.T(), ok)
			assert.NotNil(s.T(), ctags["kube_ownerref_kind"])
		}
	}, 5*time.Minute, 10*time.Second)
	require.NotEmpty(s.T(), traces)
	s.T().Log("Got traces", s.T().Name(), traces)

	// Verify tags on traces and spans
	tp := traces[0].TracerPayloads[0]
	assert.Equal(s.T(), env, tp.Env)
	assert.Equal(s.T(), version, tp.AppVersion)
	require.NotEmpty(s.T(), tp.Chunks)
	require.NotEmpty(s.T(), tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(s.T(), CalendarService, sp.Service)
		assert.Equal(s.T(), env, sp.Meta["env"])
		assert.Equal(s.T(), version, sp.Meta["version"])
		assert.Equal(s.T(), customAttributeValue, sp.Meta[customAttribute])
		assert.Equal(s.T(), sp.Meta["k8s.node.name"], tp.Hostname)
		if sp.Meta["span.kind"] == "client" {
			assert.Equal(s.T(), "calendar-rest-go", sp.Meta["otel.library.name"])
		} else {
			assert.Equal(s.T(), "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp", sp.Meta["otel.library.name"])
		}
		ctags, ok := getContainerTags(s.T(), tp)
		assert.True(s.T(), ok)
		assert.Equal(s.T(), sp.Meta["k8s.container.name"], ctags["kube_container_name"])
		assert.Equal(s.T(), sp.Meta["k8s.namespace.name"], ctags["kube_namespace"])
		assert.Equal(s.T(), sp.Meta["k8s.pod.name"], ctags["pod_name"])

		// Verify container tags from infraattributes processor
		if iaParams.InfraAttributes {
			maps.Copy(ctags, sp.Meta)
			testInfraTags(s.T(), ctags, iaParams)
		}
	}
}

// TestTracesWithSpanReceiverV2 tests that OTLP traces are received through OTel pipelines as expected with updated OTLP span receiver
func TestTracesWithSpanReceiverV2(s OTelTestSuite) {
	var err error
	var traces []*aggregator.TracePayload
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		if !assert.NoError(c, err) {
			s.T().Log("Error getting traces", s.T().Name(), err)
			return
		}
		if !assert.NotEmpty(c, traces) {
			s.T().Log("Traces empty", s.T().Name())
			return
		}
		trace := traces[0]
		if !assert.NotEmpty(s.T(), trace.TracerPayloads) {
			return
		}
		tp := trace.TracerPayloads[0]
		if !assert.NotEmpty(s.T(), tp.Chunks) {
			return
		}
		if !assert.NotEmpty(s.T(), tp.Chunks[0].Spans) {
			return
		}
		assert.Equal(s.T(), CalendarService, tp.Chunks[0].Spans[0].Service)
	}, 5*time.Minute, 10*time.Second)
	require.NotEmpty(s.T(), traces)
	s.T().Log("Got traces", s.T().Name(), traces)

	// Verify tags on traces and spans
	tp := traces[0].TracerPayloads[0]
	assert.Equal(s.T(), env, tp.Env)
	assert.Equal(s.T(), version, tp.AppVersion)
	require.NotEmpty(s.T(), tp.Chunks)
	require.NotEmpty(s.T(), tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans
	ctags, ok := getContainerTags(s.T(), tp)

	for _, sp := range spans {
		assert.Equal(s.T(), CalendarService, sp.Service)
		assert.Equal(s.T(), env, sp.Meta["env"])
		assert.Equal(s.T(), version, sp.Meta["version"])
		assert.Equal(s.T(), customAttributeValue, sp.Meta[customAttribute])
		if sp.Meta["span.kind"] == "client" {
			assert.Equal(s.T(), "client.request", sp.Name)
			assert.Equal(s.T(), "getDate", sp.Resource)
			assert.Equal(s.T(), "http", sp.Type)
			assert.IsType(s.T(), uint64(0), sp.ParentID)
			assert.NotZero(s.T(), sp.ParentID)
			assert.Equal(s.T(), "calendar-rest-go", sp.Meta["otel.library.name"])
		} else {
			assert.Equal(s.T(), "server", sp.Meta["span.kind"])
			assert.Equal(s.T(), "http.server.request", sp.Name)
			assert.Equal(s.T(), "GET", sp.Resource)
			assert.Equal(s.T(), "web", sp.Type)
			assert.Zero(s.T(), sp.ParentID)
			assert.Equal(s.T(), "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp", sp.Meta["otel.library.name"])
		}
		assert.IsType(s.T(), uint64(0), sp.TraceID)
		assert.NotZero(s.T(), sp.TraceID)
		assert.IsType(s.T(), uint64(0), sp.SpanID)
		assert.NotZero(s.T(), sp.SpanID)
		assert.Equal(s.T(), sp.Meta["k8s.node.name"], tp.Hostname)
		assert.True(s.T(), ok)
		assert.Equal(s.T(), sp.Meta["k8s.container.name"], ctags["kube_container_name"])
		assert.Equal(s.T(), sp.Meta["k8s.namespace.name"], ctags["kube_namespace"])
		assert.Equal(s.T(), sp.Meta["k8s.pod.name"], ctags["pod_name"])
	}
}

// TestTracesWithOperationAndResourceName tests that OTLP traces are received through OTel pipelines as expected with updated operation and resource name logic
func TestTracesWithOperationAndResourceName(
	s OTelTestSuite,
	clientOperationName string,
	clientResourceName string,
	serverOperationName string,
	serverResourceName string,
) {
	var err error
	var traces []*aggregator.TracePayload
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		if !assert.NoError(c, err) {
			s.T().Log("Error getting traces", s.T().Name(), err)
			return
		}
		if !assert.NotEmpty(c, traces) {
			s.T().Log("Traces empty", s.T().Name())
			return
		}
		trace := traces[0]
		if !assert.NotEmpty(s.T(), trace.TracerPayloads) {
			return
		}
		tp := trace.TracerPayloads[0]
		if !assert.NotEmpty(s.T(), tp.Chunks) {
			return
		}
		if !assert.NotEmpty(s.T(), tp.Chunks[0].Spans) {
			return
		}
		assert.Equal(s.T(), CalendarService, tp.Chunks[0].Spans[0].Service)
	}, 5*time.Minute, 10*time.Second)
	require.NotEmpty(s.T(), traces)
	s.T().Log("Got traces", s.T().Name(), traces)

	tp := traces[0].TracerPayloads[0]
	require.NotEmpty(s.T(), tp.Chunks)
	require.NotEmpty(s.T(), tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans

	for _, sp := range spans {
		if sp.Meta["span.kind"] == "client" {
			assert.Equal(s.T(), clientOperationName, sp.Name)
			assert.Equal(s.T(), clientResourceName, sp.Resource)
		} else {
			assert.Equal(s.T(), "server", sp.Meta["span.kind"])
			assert.Equal(s.T(), serverOperationName, sp.Name)
			assert.Equal(s.T(), serverResourceName, sp.Resource)
		}
	}
}

// TestMetrics tests that OTLP metrics are received through OTel pipelines as expected
func TestMetrics(s OTelTestSuite, iaParams IAParams) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var metrics []*aggregator.MetricSeries
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		tags := []string{fmt.Sprintf("service:%v", CalendarService)}
		if iaParams.InfraAttributes {
			tags = append(tags, "kube_ownerref_kind:replicaset")
		}
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("calendar-rest-go.api.counter", fakeintake.WithTags[*aggregator.MetricSeries](tags))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second)
	s.T().Log("Got metrics", s.T().Name(), metrics)

	for _, metricSeries := range metrics {
		tags := getTagMapFromSlice(s.T(), metricSeries.Tags)
		assert.Equal(s.T(), CalendarService, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])
		assert.Equal(s.T(), tags["k8s.container.name"], tags["kube_container_name"])
		assert.Equal(s.T(), tags["k8s.namespace.name"], tags["kube_namespace"])
		assert.Equal(s.T(), tags["k8s.pod.name"], tags["pod_name"])
		hasHostResource := false
		for _, resource := range metricSeries.Resources {
			if resource.Type == "host" {
				assert.Equal(s.T(), tags["k8s.node.name"], resource.Name)
				hasHostResource = true
			}
		}
		assert.True(s.T(), hasHostResource)

		// Verify container tags from infraattributes processor
		if iaParams.InfraAttributes {
			testInfraTags(s.T(), tags, iaParams)
		}
	}
}

// TestLogs tests that OTLP logs are received through OTel pipelines as expected
func TestLogs(s OTelTestSuite, iaParams IAParams) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var logs []*aggregator.Log
	s.T().Log("Waiting for logs")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		if iaParams.InfraAttributes {
			logs, err = s.Env().FakeIntake.Client().FilterLogs(CalendarService, fakeintake.WithMessageContaining(log2Body), fakeintake.WithTags[*aggregator.Log]([]string{"kube_ownerref_kind:replicaset"}))
		} else {
			logs, err = s.Env().FakeIntake.Client().FilterLogs(CalendarService, fakeintake.WithMessageContaining(log2Body))
		}
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 5*time.Minute, 10*time.Second)
	for _, l := range logs {
		s.T().Log("Got log", l)
	}

	require.NotEmpty(s.T(), logs)
	for _, log := range logs {
		tags := getLogTagsAndAttrs(s.T(), log)
		assert.Contains(s.T(), log.Message, log2Body)
		assert.Equal(s.T(), CalendarService, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])
		assert.Equal(s.T(), tags["k8s.node.name"], log.HostName)
		assert.Equal(s.T(), tags["k8s.container.name"], tags["kube_container_name"])
		assert.Equal(s.T(), tags["k8s.namespace.name"], tags["kube_namespace"])
		assert.Equal(s.T(), tags["k8s.pod.name"], tags["pod_name"])

		// Verify container tags from infraattributes processor
		if iaParams.InfraAttributes {
			testInfraTags(s.T(), tags, iaParams)
		}
	}
}

// TestHosts verifies that OTLP traces, metrics, and logs have consistent hostnames
func TestHosts(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var traces []*aggregator.TracePayload
	var metrics []*aggregator.MetricSeries
	var logs []*aggregator.Log
	s.T().Log("Waiting for telemetry")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)

		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("calendar-rest-go.api.counter", fakeintake.WithTags[*aggregator.MetricSeries]([]string{fmt.Sprintf("service:%v", CalendarService)}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)

		logs, err = s.Env().FakeIntake.Client().FilterLogs(CalendarService, fakeintake.WithMessageContaining(log2Body))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got telemetry")
	trace := traces[0]
	require.NotEmpty(s.T(), trace.TracerPayloads)
	tp := trace.TracerPayloads[0]
	traceHostname := tp.Hostname

	var metricHostname string
	metric := metrics[0]
	for _, resource := range metric.Resources {
		if resource.Type == "host" {
			metricHostname = resource.Name
		}
	}

	logHostname := logs[0].HostName

	assert.Equal(s.T(), traceHostname, metricHostname)
	assert.Equal(s.T(), traceHostname, logHostname)
	assert.Equal(s.T(), logHostname, metricHostname)
}

// TestSampling tests that APM stats are correct when using probabilistic sampling
func TestSampling(s OTelTestSuite, computeTopLevelBySpanKind bool) {
	s.T().Log("Waiting for APM stats")
	var stats []*aggregator.APMStatsPayload
	var err error
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		stats, err = s.Env().FakeIntake.Client().GetAPMStats()
		require.NoError(c, err)
		require.NotEmpty(c, stats)
		hasStatsForService := false
		for _, payload := range stats {
			for _, csp := range payload.StatsPayload.Stats {
				for _, bucket := range csp.Stats {
					for _, cgs := range bucket.Stats {
						if cgs.Service == CalendarService {
							hasStatsForService = true
							// TODO: Add functionality in example app to verify exact number of hits
							require.True(c, cgs.Hits > 0)
							if computeTopLevelBySpanKind && cgs.SpanKind == "client" {
								require.EqualValues(c, cgs.TopLevelHits, 0)
							}
							if (computeTopLevelBySpanKind && cgs.SpanKind == "server") || cgs.Resource == calendarTopLevelResource {
								require.EqualValues(c, cgs.Hits, cgs.TopLevelHits)
							}
						}
					}
				}
			}
		}
		require.True(c, hasStatsForService)
	}, 5*time.Minute, 10*time.Second)
	s.T().Log("Got APM stats", stats)
}

const (
	originProductDatadogExporter     = 19
	originServicePrometheusReceiver  = 238
	originServiceHostmetricsReceiver = 224
)

// TestPrometheusMetrics tests that expected prometheus metrics are scraped
func TestPrometheusMetrics(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var otelcolMetrics []*aggregator.MetricSeries
	var traceAgentMetrics []*aggregator.MetricSeries
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		otelcolMetrics, err = s.Env().FakeIntake.Client().FilterMetrics("otelcol_process_uptime")
		assert.NoError(c, err)
		assert.NotEmpty(c, otelcolMetrics)
		for _, m := range otelcolMetrics {
			origin := m.Metadata.Origin
			assert.Equal(c, originProductDatadogExporter, int(origin.OriginProduct))
			assert.Equal(c, originServicePrometheusReceiver, int(origin.OriginService))
		}

		traceAgentMetrics, err = s.Env().FakeIntake.Client().FilterMetrics("otelcol_datadog_trace_agent_trace_writer_spans")
		assert.NoError(c, err)
		assert.NotEmpty(c, traceAgentMetrics)
		for _, m := range otelcolMetrics {
			origin := m.Metadata.Origin
			assert.Equal(c, originProductDatadogExporter, int(origin.OriginProduct))
			assert.Equal(c, originServicePrometheusReceiver, int(origin.OriginService))
		}
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got otelcol_process_uptime", otelcolMetrics)
	s.T().Log("Got otelcol_datadog_trace_agent_trace_writer_spans", traceAgentMetrics)
}

// TestHostMetrics tests that expected host metrics are scraped
func TestHostMetrics(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	s.T().Log("Waiting for metrics")
	expectedMetrics := []string{
		"otel.system.cpu.load_average.15m",
		"otel.system.cpu.load_average.5m",
		"otel.system.memory.usage",
	}
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for _, m := range expectedMetrics {
			metrics, err := s.Env().FakeIntake.Client().FilterMetrics(m)
			assert.NoError(c, err)
			s.T().Log("Got host metric", metrics)
			assert.NotEmpty(c, metrics)
			for _, metric := range metrics {
				origin := metric.Metadata.Origin
				assert.Equal(c, originProductDatadogExporter, int(origin.OriginProduct))
				assert.Equal(c, originServiceHostmetricsReceiver, int(origin.OriginService))
			}
		}
	}, 1*time.Minute, 10*time.Second)
}

func getLoadBalancingSpans(t require.TestingT, traces []*aggregator.TracePayload) map[string][]*trace.Span {
	spanMap := make(map[string][]*trace.Span)
	spans := 0
	for _, tracePayload := range traces {
		for _, tracerPayload := range tracePayload.TracerPayloads {
			for _, chunk := range tracerPayload.Chunks {
				for _, span := range chunk.Spans {
					if len(spanMap[span.Service]) < 3 {
						spanMap[span.Service] = append(spanMap[span.Service], span)
						spans++
					}
					if spans == 12 {
						return spanMap
					}
				}
			}
		}
	}
	require.Equal(t, 12, spans)
	return spanMap
}

func getLoadBalancingLogs(c require.TestingT, s OTelTestSuite, service string) {
	logs1, err := s.Env().FakeIntake.Client().FilterLogs(service, fakeintake.WithMessageMatching(log1Body))
	require.NoError(c, err)
	require.NotEmpty(c, logs1)
	log1 := logs1[0]
	log1Tags := getLogTagsAndAttrs(c, log1)

	logs2, err := s.Env().FakeIntake.Client().FilterLogs(service, fakeintake.WithMessageMatching(log2Body))
	require.NoError(c, err)
	require.NotEmpty(c, logs2)
	matchedLog := false
	for _, log2 := range logs2 {
		// Find second log with the same trace id
		log2Tags := getLogTagsAndAttrs(c, log2)
		if log1Tags["message"] != log2Tags["message"] && log1Tags["dd.trace_id"] == log2Tags["dd.trace_id"] {
			// Verify that logs with the same trace id are sent to the same backend
			s.T().Log("Log service", service+",", "Log trace_id", log1Tags["dd.trace_id"]+",", "Log1 Backend", log1Tags["backend"]+",", "Log2 Backend", log2Tags["backend"])
			require.Equal(c, log1Tags["backend"], log2Tags["backend"])
			matchedLog = true
			break
		}
	}
	require.True(c, matchedLog)
}

func getLoadBalancingMetrics(t require.TestingT, metrics []*aggregator.MetricSeries) map[string][]map[string]string {
	metricTagsMap := make(map[string][]map[string]string)
	ms := 0
	for _, metricSeries := range metrics {
		tags := getTagMapFromSlice(t, metricSeries.Tags)
		service := tags["service"]
		if len(metricTagsMap[service]) < 3 {
			metricTagsMap[service] = append(metricTagsMap[service], tags)
			ms++
		}
		if ms == 12 {
			return metricTagsMap
		}
	}
	require.Equal(t, 12, ms)
	return metricTagsMap
}

// TestLoadBalancing verifies that the loadbalancingexporter correctly routes traces and metrics by service
func TestLoadBalancing(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	var spanMap map[string][]*trace.Span
	var metricTagsMap map[string][]map[string]string

	s.T().Log("Waiting for telemetry")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		require.NoError(c, err)
		require.NotEmpty(c, traces)
		spanMap = getLoadBalancingSpans(c, traces)

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("calendar-rest-go.api.counter")
		require.NoError(c, err)
		require.NotEmpty(c, metrics)
		metricTagsMap = getLoadBalancingMetrics(c, metrics)

		for service := range spanMap {
			getLoadBalancingLogs(c, s, service)
		}
	}, 5*time.Minute, 10*time.Second)
	s.T().Log("Got telemetry", s.T().Name())
	for service, spans := range spanMap {
		backend := ""
		for _, span := range spans {
			s.T().Log("Span service:", service+",", "Backend:", span.Meta["backend"])
			if backend == "" {
				backend = span.Meta["backend"]
				continue
			}
			assert.Equal(s.T(), backend, span.Meta["backend"])
		}
	}
	for service, metricTags := range metricTagsMap {
		backend := ""
		for _, tags := range metricTags {
			s.T().Log("Metric service:", service+",", "Backend:", tags["backend"])
			if backend == "" {
				backend = tags["backend"]
				continue
			}
			assert.Equal(s.T(), backend, tags["backend"])
		}
	}
}

// TestCalendarApp starts the calendar app to send telemetry for e2e tests
func TestCalendarApp(s OTelTestSuite, ust bool, service string) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Log("Starting calendar app:", service)
	createCalendarApp(ctx, s, ust, service)

	// Wait for calendar app to start
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(service, fakeintake.WithMessageContaining(log2Body))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 30*time.Minute, 10*time.Second)
}

func createCalendarApp(ctx context.Context, s OTelTestSuite, ust bool, service string) {
	var replicas int32 = 1
	name := fmt.Sprintf("%v-%v", service, strings.ReplaceAll(strings.ToLower(s.T().Name()), "/", "-"))

	otlpEndpoint := fmt.Sprintf("http://%v:4317", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"])
	serviceSpec := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"helm.sh/chart":                "calendar-rest-go-0.1.0",
				"app.kubernetes.io/name":       name,
				"app.kubernetes.io/instance":   name,
				"app.kubernetes.io/version":    "0.15",
				"app.kubernetes.io/managed-by": "Helm",
			},
			Annotations: map[string]string{
				"openshift.io/deployment.name": "openshift",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       9090,
					TargetPort: intstr.FromString("http"),
					Protocol:   "TCP",
					Name:       "http",
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":     name,
				"app.kubernetes.io/instance": name,
			},
		},
	}
	deploymentSpec := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"helm.sh/chart":                "calendar-rest-go-0.1.0",
				"app.kubernetes.io/name":       name,
				"app.kubernetes.io/instance":   name,
				"app.kubernetes.io/version":    "0.15",
				"app.kubernetes.io/managed-by": "Helm",
			},
			Annotations: map[string]string{
				"openshift.io/deployment.name": "openshift",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     name,
					"app.kubernetes.io/instance": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"helm.sh/chart":                "calendar-rest-go-0.1.0",
						"app.kubernetes.io/name":       name,
						"app.kubernetes.io/instance":   name,
						"app.kubernetes.io/version":    "0.15",
						"app.kubernetes.io/managed-by": "Helm",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            name,
						Image:           "ghcr.io/datadog/apps-calendar-go:" + apps.Version,
						ImagePullPolicy: "IfNotPresent",
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: 9090,
						}},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/calendar",
									Port: intstr.FromString("http"),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/calendar",
									Port: intstr.FromString("http"),
								},
							},
						},
						Env: getCalendarAppEnvVars(name, otlpEndpoint, ust, service),
					},
					},
				},
			},
		},
	}

	_, err := s.Env().KubernetesCluster.Client().CoreV1().Services("datadog").Create(ctx, serviceSpec, metav1.CreateOptions{})
	require.NoError(s.T(), err, "Could not properly start service")
	_, err = s.Env().KubernetesCluster.Client().AppsV1().Deployments("datadog").Create(ctx, deploymentSpec, metav1.CreateOptions{})
	require.NoError(s.T(), err, "Could not properly start deployment")
}

func getCalendarAppEnvVars(name string, otlpEndpoint string, ust bool, service string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{{
		Name:  "OTEL_CONTAINER_NAME",
		Value: name,
	}, {
		Name:      "OTEL_K8S_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
	}, {
		Name:      "OTEL_K8S_NODE_NAME",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
	}, {
		Name:      "OTEL_K8S_POD_NAME",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
	}, {
		Name:      "OTEL_K8S_POD_ID",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}},
	}, {
		Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
		Value: otlpEndpoint,
	}, {
		Name:  "OTEL_EXPORTER_OTLP_PROTOCOL",
		Value: "grpc",
	}}
	resourceAttrs := "k8s.namespace.name=$(OTEL_K8S_NAMESPACE)," +
		"k8s.node.name=$(OTEL_K8S_NODE_NAME)," +
		"k8s.pod.name=$(OTEL_K8S_POD_NAME)," +
		"k8s.pod.uid=$(OTEL_K8S_POD_ID)," +
		"k8s.container.name=$(OTEL_CONTAINER_NAME)," +
		"host.name=$(OTEL_K8S_NODE_NAME)," +
		fmt.Sprintf("%v=%v", customAttribute, customAttributeValue)

	// Use Unified Service Tagging env vars instead of OTel env vars
	if ust {
		return append(envVars, []corev1.EnvVar{{
			Name:  "DD_SERVICE",
			Value: service,
		}, {
			Name:  "DD_ENV",
			Value: env,
		}, {
			Name:  "DD_VERSION",
			Value: version,
		}, {
			Name:  "OTEL_RESOURCE_ATTRIBUTES",
			Value: resourceAttrs,
		}}...)
	}

	return append(envVars, []corev1.EnvVar{{
		Name:  "OTEL_SERVICE_NAME",
		Value: service,
	}, {
		Name: "OTEL_RESOURCE_ATTRIBUTES",
		Value: resourceAttrs +
			fmt.Sprintf(",deployment.environment=%v,", env) +
			fmt.Sprintf("service.version=%v", version),
	}}...)
}

func testInfraTags(t *testing.T, tags map[string]string, iaParams IAParams) {
	assert.NotNil(t, tags["kube_deployment"])
	assert.NotNil(t, tags["kube_qos"])
	assert.NotNil(t, tags["kube_replica_set"])
	assert.NotNil(t, tags["pod_phase"])
	assert.Equal(t, "replicaset", tags["kube_ownerref_kind"])
	assert.Equal(t, tags["kube_app_instance"], tags["kube_app_name"])
	assert.Contains(t, tags["k8s.pod.name"], tags["kube_replica_set"])

	if iaParams.EKS {
		assert.NotNil(t, tags["container_id"])
		assert.NotNil(t, tags["image_id"])
		assert.NotNil(t, tags["image_name"])
		assert.NotNil(t, tags["image_tag"])
		assert.NotNil(t, tags["short_image"])
	}
	if iaParams.Cardinality == types.OrchestratorCardinality || iaParams.Cardinality == types.HighCardinality {
		assert.Contains(t, tags["k8s.pod.name"], tags["kube_ownerref_name"])
		assert.Equal(t, tags["kube_replica_set"], tags["kube_ownerref_name"])
	}
	if iaParams.Cardinality == types.HighCardinality && iaParams.EKS {
		assert.NotNil(t, tags["display_container_name"])
	}
}

func getContainerTags(t *testing.T, tp *trace.TracerPayload) (map[string]string, bool) {
	ctags, ok := tp.Tags["_dd.tags.container"]
	if !ok {
		return nil, false
	}
	splits := strings.Split(ctags, ",")
	return getTagMapFromSlice(t, splits), true
}

func getLogTagsAndAttrs(t require.TestingT, log *aggregator.Log) map[string]string {
	tags := getTagMapFromSlice(t, log.Tags)
	attrs := make(map[string]interface{})
	err := json.Unmarshal([]byte(log.Message), &attrs)
	require.NoError(t, err)
	for k, v := range attrs {
		tags[k] = fmt.Sprint(v)
	}
	return tags
}

func getTagMapFromSlice(t assert.TestingT, tagSlice []string) map[string]string {
	m := make(map[string]string)
	for _, s := range tagSlice {
		kv := strings.SplitN(s, ":", 2)
		if !assert.Len(t, kv, 2, "malformed tag: %v", s) {
			continue
		}
		m[kv[0]] = kv[1]
	}
	return m
}
