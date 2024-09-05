// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

const (
	service              = "telemetrygen-job"
	env                  = "e2e"
	version              = "1.0"
	customAttribute      = "custom.attribute"
	customAttributeValue = "true"
)

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed collector.yml
var collectorConfig string

func TestOTel(t *testing.T) {
	e2e.Run(t, &linuxTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(collectorConfig)))))
}

func (s *linuxTestSuite) TestOTelAgentInstalled() {
	agent := s.getAgentPod()
	assert.Contains(s.T(), agent.ObjectMeta.String(), "otel-agent")
}

func (s *linuxTestSuite) TestOTLPTraces() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "traces", []string{"--traces", fmt.Sprint(numTraces)})

	var traces []*aggregator.TracePayload
	var err error
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
	}, 2*time.Minute, 10*time.Second)
	require.NotEmpty(s.T(), traces)
	s.T().Log("Got traces", traces)
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
		s.T().Log("k8s.node.name", sp.Meta["k8s.node.name"])
		s.T().Log("tp.Hostname", tp.Hostname)

		// Verify container tags from infraattributes processor
		assert.NotNil(s.T(), ctags["kube_container_name"])
		assert.NotNil(s.T(), ctags["kube_namespace"])
		assert.NotNil(s.T(), ctags["pod_name"])
		assert.Equal(s.T(), sp.Meta["k8s.container.name"], ctags["kube_container_name"])
		assert.Equal(s.T(), sp.Meta["k8s.namespace.name"], ctags["kube_namespace"])
		assert.Equal(s.T(), sp.Meta["k8s.pod.name"], ctags["pod_name"])
	}

	s.T().Log("Waiting for APM stats")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		stats, err := s.Env().FakeIntake.Client().GetAPMStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
		s.T().Log("Got APM stats", stats)
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
}

func (s *linuxTestSuite) TestOTLPMetrics() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	numMetrics := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "metrics", []string{"--metrics", fmt.Sprint(numMetrics)})

	var metrics []*aggregator.MetricSeries
	var err error
	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("gen", fakeintake.WithTags[*aggregator.MetricSeries]([]string{fmt.Sprintf("service:%v", service)}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
		s.T().Log("Got metrics", metrics)
	}, 2*time.Minute, 10*time.Second)

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
				s.T().Log("k8s.node.name", tags["k8s.node.name"])
				s.T().Log("resource.Name", resource.Name)
				hasHostResource = true
			}
		}
		assert.True(s.T(), hasHostResource)
	}
}

func (s *linuxTestSuite) TestOTLPLogs() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	numLogs := 10
	logBody := "telemetrygen log"

	s.T().Log("Starting telemetrygen")
	ddtags := fmt.Sprintf("ddtags=\"k8s.namespace.name:$(OTEL_K8S_NAMESPACE),k8s.node.name:$(OTEL_K8S_NODE_NAME),k8s.pod.name:$(OTEL_K8S_POD_NAME),k8s.container.name:telemetrygen-job,%v:%v\"", customAttribute, customAttributeValue)
	s.createTelemetrygenJob(ctx, "logs", []string{"--logs", fmt.Sprint(numLogs), "--body", logBody, "--telemetry-attributes", ddtags})

	var logs []*aggregator.Log
	var err error
	s.T().Log("Waiting for logs")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err = s.Env().FakeIntake.Client().FilterLogs(service, fakeintake.WithMessageContaining(logBody))
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 2*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), logs)
	for _, log := range logs {
		assert.Contains(s.T(), log.Message, logBody)
		s.T().Log("log.Tags", log.Tags)
		tags := getTagMapFromSlice(s.T(), log.Tags)
		s.T().Log("tags", tags)
		assert.Equal(s.T(), service, tags["service"])
		assert.Equal(s.T(), env, tags["env"])
		assert.Equal(s.T(), version, tags["version"])
		assert.Equal(s.T(), customAttributeValue, tags[customAttribute])
		assert.Equal(s.T(), tags["k8s.node.name"], log.HostName)
		s.T().Log("k8s.node.name", tags["k8s.node.name"])
		s.T().Log("log.HostName", log.HostName)

		// Verify container tags from infraattributes processor
		assert.NotNil(s.T(), tags["kube_container_name"])
		assert.NotNil(s.T(), tags["kube_namespace"])
		assert.NotNil(s.T(), tags["pod_name"])
		assert.Equal(s.T(), tags["k8s.container.name"], tags["kube_container_name"])
		assert.Equal(s.T(), tags["k8s.namespace.name"], tags["kube_namespace"])
		assert.Equal(s.T(), tags["k8s.pod.name"], tags["pod_name"])
		s.T().Log("source", log.Source)
		s.T().Log("status", log.Status)
	}

	time.Sleep(30 * time.Second)
}

func (s *linuxTestSuite) TestOTelFlare() {
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	s.T().Log("Starting flare")
	agent := s.getAgentPod()
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"agent", "flare", "--email", "e2e@test.com", "--send"})
	require.NoError(s.T(), err, "Failed to execute flare")
	require.Empty(s.T(), stderr)
	require.NotNil(s.T(), stdout)

	s.T().Log("Getting latest flare")
	flare, err := s.Env().FakeIntake.Client().GetLatestFlare()
	require.NoError(s.T(), err, "Failed to get latest flare")
	otelFolder, otelFlareFolder := false, false
	var otelResponse string
	for _, filename := range flare.GetFilenames() {
		if strings.Contains(filename, "/otel/") {
			otelFolder = true
		}
		if strings.Contains(filename, "/otel/otel-flare/") {
			otelFlareFolder = true
		}
		if strings.Contains(filename, "otel/otel-response.json") {
			otelResponse = filename
		}
	}
	assert.True(s.T(), otelFolder)
	assert.True(s.T(), otelFlareFolder)
	otelResponseContent, err := flare.GetFileContent(otelResponse)
	require.NoError(s.T(), err)
	expectedContents := []string{"otel-agent", "ddflare/dd-autoconfigured:", "health_check/dd-autoconfigured:", "pprof/dd-autoconfigured:", "zpages/dd-autoconfigured:", "infraattributes/dd-autoconfigured:", "prometheus/dd-autoconfigured:", "key: '[REDACTED]'"}
	for _, expected := range expectedContents {
		assert.Contains(s.T(), otelResponseContent, expected)
	}
}

func (s *linuxTestSuite) getAgentPod() corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items)
	return res.Items[0]
}

func (s *linuxTestSuite) createTelemetrygenJob(ctx context.Context, telemetry string, options []string) {
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
