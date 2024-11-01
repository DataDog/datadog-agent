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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

const (
	calendarService              = "calendar-rest-go"
	telemetrygenService          = "telemetrygen-job"
	telemetrygenTopLevelResource = "lets-go"
	env                          = "e2e"
	version                      = "1.0"
	customAttribute              = "custom.attribute"
	customAttributeValue         = "true"
	logBody                      = "random date"
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
		assert.Equal(s.T(), calendarService, tp.Chunks[0].Spans[0].Service)
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
		assert.Equal(s.T(), calendarService, sp.Service)
		assert.Equal(s.T(), env, sp.Meta["env"])
		assert.Equal(s.T(), version, sp.Meta["version"])
		assert.Equal(s.T(), customAttributeValue, sp.Meta[customAttribute])
		assert.Equal(s.T(), "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp", sp.Meta["otel.library.name"])
		assert.Equal(s.T(), sp.Meta["k8s.node.name"], tp.Hostname)
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

// TestMetrics tests that OTLP metrics are received through OTel pipelines as expected
func TestMetrics(s OTelTestSuite, iaParams IAParams) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var metrics []*aggregator.MetricSeries
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		tags := []string{fmt.Sprintf("service:%v", calendarService)}
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
		assert.Equal(s.T(), calendarService, tags["service"])
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
			logs, err = s.Env().FakeIntake.Client().FilterLogs(calendarService, fakeintake.WithMessageContaining(logBody), fakeintake.WithTags[*aggregator.Log]([]string{"kube_ownerref_kind:replicaset"}))
		} else {
			logs, err = s.Env().FakeIntake.Client().FilterLogs(calendarService, fakeintake.WithMessageContaining(logBody))
		}
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 5*time.Minute, 10*time.Second)
	for _, l := range logs {
		s.T().Log("Got log", l)
	}

	require.NotEmpty(s.T(), logs)
	for _, log := range logs {
		tags := getTagMapFromSlice(s.T(), log.Tags)
		attrs := make(map[string]interface{})
		err = json.Unmarshal([]byte(log.Message), &attrs)
		assert.NoError(s.T(), err)
		for k, v := range attrs {
			tags[k] = fmt.Sprint(v)
		}
		assert.Contains(s.T(), log.Message, logBody)
		assert.Equal(s.T(), calendarService, tags["service"])
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

		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("calendar-rest-go.api.counter", fakeintake.WithTags[*aggregator.MetricSeries]([]string{fmt.Sprintf("service:%v", calendarService)}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)

		logs, err = s.Env().FakeIntake.Client().FilterLogs(calendarService, fakeintake.WithMessageContaining(logBody))
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
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	createTelemetrygenJob(ctx, s, "traces", []string{"--traces", fmt.Sprint(numTraces)})

	TestAPMStats(s, numTraces, computeTopLevelBySpanKind)
}

// TestAPMStats checks that APM stats are received with the correct number of hits per traces given
func TestAPMStats(s OTelTestSuite, numTraces int, computeTopLevelBySpanKind bool) {
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
						if cgs.Service == telemetrygenService {
							hasStatsForService = true
							assert.EqualValues(c, cgs.Hits, numTraces)
							if computeTopLevelBySpanKind || cgs.Resource == telemetrygenTopLevelResource {
								assert.EqualValues(c, cgs.TopLevelHits, numTraces)
							}
						}
					}
				}
			}
		}
		assert.True(c, hasStatsForService)
	}, 2*time.Minute, 10*time.Second)
	s.T().Log("Got APM stats", stats)
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
			Name:      fmt.Sprintf("telemetrygen-job-%v-%v", telemetry, strings.ReplaceAll(strings.ToLower(s.T().Name()), "/", "-")),
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
								Value: telemetrygenService,
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

// TestCalendarApp tests that OTLP metrics are received through OTel pipelines as expected
func TestCalendarApp(s OTelTestSuite) {
	ctx := context.Background()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Log("Starting calendar app")
	createCalendarApp(ctx, s)

	// Wait for calendar app to start
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(calendarService, fakeintake.WithMessageContaining(logBody))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 30*time.Minute, 10*time.Second)
}

func createCalendarApp(ctx context.Context, s OTelTestSuite) {
	var replicas int32 = 1
	name := fmt.Sprintf("calendar-rest-go-%v", strings.ReplaceAll(strings.ToLower(s.T().Name()), "/", "-"))

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
					Port: 9090,
					TargetPort: intstr.IntOrString{
						StrVal: "http",
					},
					Protocol: "TCP",
					Name:     "http",
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
						Image:           "ghcr.io/datadog/apps-calendar-go:main",
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
						Env: []corev1.EnvVar{{
							Name:  "OTEL_SERVICE_NAME",
							Value: calendarService,
						}, {
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
						}, {
							Name: "OTEL_RESOURCE_ATTRIBUTES",
							Value: "service.name=$(OTEL_SERVICE_NAME)," +
								"k8s.namespace.name=$(OTEL_K8S_NAMESPACE)," +
								"k8s.node.name=$(OTEL_K8S_NODE_NAME)," +
								"k8s.pod.name=$(OTEL_K8S_POD_NAME)," +
								"k8s.pod.uid=$(OTEL_K8S_POD_ID)," +
								"k8s.container.name=$(OTEL_CONTAINER_NAME)," +
								"host.name=$(OTEL_K8S_NODE_NAME)," +
								fmt.Sprintf("deployment.environment=%v,", env) +
								fmt.Sprintf("service.version=%v,", version) +
								fmt.Sprintf("%v=%v", customAttribute, customAttributeValue),
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

func getTagMapFromSlice(t *testing.T, tagSlice []string) map[string]string {
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
