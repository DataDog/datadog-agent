// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains util functions for OTel e2e tests
package utils

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

const (
	service              = "telemetrygen-job"
	env                  = "e2e"
	version              = "1.0"
	customAttribute      = "custom.attribute"
	customAttributeValue = "true"
)

// OTelTestSuite is an interface for the OTel e2e test suite.
type OTelTestSuite interface {
	T() *testing.T
	Env() *environments.Kubernetes
}

// TestTraces tests that OTLP traces are received through OTel pipelines as expected
func TestTraces(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	createTelemetrygenJob(ctx, s, "traces", []string{"--traces", fmt.Sprint(numTraces)})

	var traces []*aggregator.TracePayload
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
	}, 2*time.Minute, 10*time.Second)
	require.NotEmpty(s.T(), traces)
	s.T().Log("Got traces", traces)
	s.T().Log("num traces", len(traces))
	trace := traces[0]
	require.NotEmpty(s.T(), trace.TracerPayloads)

	// Verify tags on traces and spans
	tp := trace.TracerPayloads[0]
	ctags, ok := getContainerTags(s.T(), tp)
	require.True(s.T(), ok)
	assert.Equal(s.T(), env, tp.Env)
	assert.Equal(s.T(), version, tp.AppVersion)
	require.NotEmpty(s.T(), tp.Chunks)
	require.NotEmpty(s.T(), tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(s.T(), service, sp.Service)
		assert.Equal(s.T(), env, sp.Meta["env"])
		assert.Equal(s.T(), version, sp.Meta["version"])
		assert.Equal(s.T(), customAttributeValue, sp.Meta[customAttribute])
		assert.Equal(s.T(), "telemetrygen", sp.Meta["otel.library.name"])
		assert.Equal(s.T(), sp.Meta["k8s.node.name"], tp.Hostname)

		// Verify container tags from infraattributes processor
		assert.NotNil(s.T(), ctags["kube_container_name"])
		assert.NotNil(s.T(), ctags["kube_namespace"])
		assert.NotNil(s.T(), ctags["pod_name"])
		assert.Equal(s.T(), sp.Meta["k8s.container.name"], ctags["kube_container_name"])
		assert.Equal(s.T(), sp.Meta["k8s.namespace.name"], ctags["kube_namespace"])
		assert.Equal(s.T(), sp.Meta["k8s.pod.name"], ctags["pod_name"])
	}
	TestAPMStats(s, numTraces)
}

// TestAPMStats checks that APM stats are received with the correct number of hits per traces given
func TestAPMStats(s OTelTestSuite, numTraces int) {
	s.T().Log("Waiting for APM stats")
	var stats []*aggregator.APMStatsPayload
	var err error
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		stats, err = s.Env().FakeIntake.Client().GetAPMStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
		hasStatsForService := false
		for _, payload := range stats {
			for _, csp := range payload.StatsPayload.Stats {
				for _, bucket := range csp.Stats {
					for _, cgs := range bucket.Stats {
						if cgs.Service == service {
							hasStatsForService = true
							assert.EqualValues(c, cgs.Hits, numTraces)
							assert.EqualValues(c, cgs.TopLevelHits, numTraces)
						}
					}
				}
			}
		}
		assert.True(c, hasStatsForService)
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got APM stats", stats)
}

// TestMetrics tests that OTLP metrics are received through OTel pipelines as expected
func TestMetrics(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	numMetrics := 10

	s.T().Log("Starting telemetrygen")
	createTelemetrygenJob(ctx, s, "metrics", []string{"--metrics", fmt.Sprint(numMetrics)})

	var metrics []*aggregator.MetricSeries
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("gen", fakeintake.WithTags[*aggregator.MetricSeries]([]string{fmt.Sprintf("service:%v", service)}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got metrics", metrics)

	for _, metricSeries := range metrics {
		tags := getTagMapFromSlice(s.T(), metricSeries.Tags)
		assert.Equal(s.T(), service, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])

		// Verify container tags from infraattributes processor
		assert.NotNil(s.T(), tags["kube_container_name"])
		assert.NotNil(s.T(), tags["kube_namespace"])
		assert.NotNil(s.T(), tags["pod_name"])
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
	}
}

// TestLogs tests that OTLP logs are received through OTel pipelines as expected
func TestLogs(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	numLogs := 10
	logBody := "telemetrygen log"

	s.T().Log("Starting telemetrygen")
	ddtags := fmt.Sprintf("ddtags=\"k8s.namespace.name:$(OTEL_K8S_NAMESPACE),k8s.node.name:$(OTEL_K8S_NODE_NAME),k8s.pod.name:$(OTEL_K8S_POD_NAME),k8s.container.name:telemetrygen-job,%v:%v\"", customAttribute, customAttributeValue)
	createTelemetrygenJob(ctx, s, "logs", []string{"--logs", fmt.Sprint(numLogs), "--body", logBody, "--telemetry-attributes", ddtags})

	var logs []*aggregator.Log
	s.T().Log("Waiting for logs")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err = s.Env().FakeIntake.Client().FilterLogs(service, fakeintake.WithMessageContaining(logBody))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got logs", logs)

	require.NotEmpty(s.T(), logs)
	for _, log := range logs {
		assert.Contains(s.T(), log.Message, logBody)
		tags := getTagMapFromSlice(s.T(), log.Tags)
		assert.Equal(s.T(), service, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])
		assert.Equal(s.T(), tags["k8s.node.name"], log.HostName)

		// Verify container tags from infraattributes processor
		assert.NotNil(s.T(), tags["kube_container_name"])
		assert.NotNil(s.T(), tags["kube_namespace"])
		assert.NotNil(s.T(), tags["pod_name"])
		assert.Equal(s.T(), tags["k8s.container.name"], tags["kube_container_name"])
		assert.Equal(s.T(), tags["k8s.namespace.name"], tags["kube_namespace"])
		assert.Equal(s.T(), tags["k8s.pod.name"], tags["pod_name"])
	}
}

// TestHosts verifies that OTLP traces, metrics, and logs have consistent hostnames
func TestHosts(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	numTelemetry := 10
	logBody := "telemetrygen log"

	s.T().Log("Starting telemetrygen traces")
	createTelemetrygenJob(ctx, s, "traces", []string{"--traces", fmt.Sprint(numTelemetry)})
	s.T().Log("Starting telemetrygen metrics")
	createTelemetrygenJob(ctx, s, "metrics", []string{"--metrics", fmt.Sprint(numTelemetry)})
	s.T().Log("Starting telemetrygen logs")
	createTelemetrygenJob(ctx, s, "logs", []string{"--logs", fmt.Sprint(numTelemetry), "--body", logBody})

	var traces []*aggregator.TracePayload
	var metrics []*aggregator.MetricSeries
	var logs []*aggregator.Log
	s.T().Log("Waiting for telemetry")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)

		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("gen", fakeintake.WithTags[*aggregator.MetricSeries]([]string{fmt.Sprintf("service:%v", service)}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)

		logs, err = s.Env().FakeIntake.Client().FilterLogs(service, fakeintake.WithMessageContaining(logBody))
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
func TestSampling(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	createTelemetrygenJob(ctx, s, "traces", []string{"--traces", fmt.Sprint(numTraces)})

	TestAPMStats(s, numTraces)
}

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

		traceAgentMetrics, err = s.Env().FakeIntake.Client().FilterMetrics("otelcol_datadog_trace_agent_trace_writer_spans")
		assert.NoError(c, err)
		assert.NotEmpty(c, traceAgentMetrics)
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got otelcol_process_uptime", otelcolMetrics)
	s.T().Log("Got otelcol_datadog_trace_agent_trace_writer_spans", traceAgentMetrics)
}

func createTelemetrygenJob(ctx context.Context, s OTelTestSuite, telemetry string, options []string) {
	var ttlSecondsAfterFinished int32 = 0 //nolint:revive // We want to see this is explicitly set to 0
	var backOffLimit int32 = 4

	otlpEndpoint := fmt.Sprintf("%v:4317", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"])
	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("telemetrygen-job-%v", telemetry),
			Namespace: "datadog",
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{{
								Name:  "OTEL_SERVICE_NAME",
								Value: service,
							}, {
								Name:      "OTEL_K8S_POD_ID",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}},
							}, {
								Name:      "OTEL_K8S_NAMESPACE",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
							}, {
								Name:      "OTEL_K8S_NODE_NAME",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
							}, {
								Name:      "OTEL_K8S_POD_NAME",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
							}},
							Name:  "telemetrygen-job",
							Image: "ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:v0.107.0",
							Command: append([]string{
								"/telemetrygen", telemetry, "--otlp-endpoint", otlpEndpoint, "--otlp-insecure",
								"--telemetry-attributes", fmt.Sprintf("%v=%v", customAttribute, customAttributeValue),
								"--otlp-attributes", "service.name=\"$(OTEL_SERVICE_NAME)\"",
								"--otlp-attributes", "host.name=\"$(OTEL_K8S_NODE_NAME)\"",
								"--otlp-attributes", fmt.Sprintf("deployment.environment=\"%v\"", env),
								"--otlp-attributes", fmt.Sprintf("service.version=\"%v\"", version),
								"--otlp-attributes", "k8s.namespace.name=\"$(OTEL_K8S_NAMESPACE)\"",
								"--otlp-attributes", "k8s.node.name=\"$(OTEL_K8S_NODE_NAME)\"",
								"--otlp-attributes", "k8s.pod.name=\"$(OTEL_K8S_POD_NAME)\"",
								"--otlp-attributes", "k8s.pod.uid=\"$(OTEL_K8S_POD_ID)\"",
								"--otlp-attributes", "k8s.container.name=\"telemetrygen-job\"",
							}, options...),
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := s.Env().KubernetesCluster.Client().BatchV1().Jobs("datadog").Create(ctx, jobSpec, metav1.CreateOptions{})
	require.NoError(s.T(), err, "Could not properly start job")
}

func createApp(ctx context.Context, s OTelTestSuite) {
	var replicas int32 = 1

	otlpEndpoint := fmt.Sprintf("otlp://%v:4317", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"])
	serviceSpec := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "manual-container-metrics-app",
			Labels: map[string]string{
				"helm.sh/chart":                "manual-container-metrics-app-0.1.0",
				"app.kubernetes.io/name":       "manual-container-metrics-app",
				"app.kubernetes.io/instance":   "manual-container-metrics-app",
				"app.kubernetes.io/version":    "1.16.0",
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
					Port: 3000,
					TargetPort: intstr.IntOrString{
						StrVal: "http",
					},
					Protocol: "TCP",
					Name:     "http",
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":     "manual-container-metrics-app",
				"app.kubernetes.io/instance": "manual-container-metrics-app",
			},
		},
	}
	deploymentSpec := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "manual-container-metrics-app",
			Labels: map[string]string{
				"helm.sh/chart":                "manual-container-metrics-app-0.1.0",
				"app.kubernetes.io/name":       "manual-container-metrics-app",
				"app.kubernetes.io/instance":   "manual-container-metrics-app",
				"app.kubernetes.io/version":    "1.16.0",
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
					"app.kubernetes.io/name":     "manual-container-metrics-app",
					"app.kubernetes.io/instance": "manual-container-metrics-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":     "manual-container-metrics-app",
						"app.kubernetes.io/instance": "manual-container-metrics-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            "manual-container-metrics-app",
						Image:           "datadog/opentelemetry-examples:manual-container-metrics-app-v1.0.5",
						ImagePullPolicy: "IfNotPresent",
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: 3000,
							Protocol:      "TCP",
						}},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/liveness",
									Port: intstr.FromString("http"),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readiness",
									Port: intstr.FromString("http"),
								},
							},
						},
						Env: []corev1.EnvVar{{
							Name:  "OTEL_SERVICE_NAME",
							Value: "manual-container-metrics-app",
						}, {
							Name:  "OTEL_CONTAINER_NAME",
							Value: "manual-container-metrics-app",
						}, {
							Name:      "OTEL_K8S_CONTAINER_ID",
							ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}},
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
							Name:  "OTEL_EXPORTER_OTLP_ENDPOINT",
							Value: otlpEndpoint,
						}, {
							Name:  "OTEL_EXPORTER_OTLP_PROTOCOL",
							Value: "grpc",
						}, {
							Name: "OTEL_RESOURCE_ATTRIBUTES",
							Value: "service.name=$(OTEL_SERVICE_NAME)," +
								//"k8s.namespace.name=$(OTEL_K8S_NAMESPACE)," +
								//"k8s.node.name=$(OTEL_K8S_NODE_NAME)," +
								//"k8s.pod.name=$(OTEL_K8S_POD_NAME)," +
								//"k8s.container.name=manual-container-metrics-app," +
								"host.name=$(OTEL_K8S_NODE_NAME)," +
								"deployment.environment=$(OTEL_K8S_NAMESPACE)," +
								//"container.name=$(OTEL_CONTAINER_NAME)," +
								"k8s.pod.uid=$(OTEL_K8S_CONTAINER_ID)",
						}},
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

// TestContainerMetrics tests that OTLP metrics are received through OTel pipelines as expected
func TestContainerMetrics(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Log("Starting app")
	createApp(ctx, s)

	var metrics []*aggregator.MetricSeries
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("container.cpu.usage", fakeintake.WithTags[*aggregator.MetricSeries]([]string{"service.name:manual-container-metrics-app"}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second)
	s.T().Log("Got metrics", metrics)
}

func getContainerTags(t *testing.T, tp *trace.TracerPayload) (map[string]string, bool) {
	ctags, ok := tp.Tags["_dd.tags.container"]
	if !ok {
		return nil, false
	}
	splits := strings.Split(ctags, ",")
	m := make(map[string]string)
	for _, s := range splits {
		kv := strings.SplitN(s, ":", 2)
		if !assert.Len(t, kv, 2, "malformed container tag: %v", s) {
			continue
		}
		m[kv[0]] = kv[1]
	}
	return m, true
}

func getTagMapFromSlice(t *testing.T, tagSlice []string) map[string]string {
	m := make(map[string]string)
	for _, s := range tagSlice {
		kv := strings.SplitN(s, ":", 2)
		require.Len(t, kv, 2, "malformed tag: %v", s)
		m[kv[0]] = kv[1]
	}
	return m
}
