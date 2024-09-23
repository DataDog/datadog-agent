// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localkubernetes contains the provisioner for the local Kubernetes based environments

package otel

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

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed collector.yml
var collectorConfig string

func TestOTel(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(collectorConfig)))))
}

func (s *linuxTestSuite) TestOTelAgentInstalled() {
	agent := s.getAgentPod()
	assert.Contains(s.T(), agent.ObjectMeta.String(), "otel-agent")
}

func (s *linuxTestSuite) TestOTLPTraces() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "traces", []string{"--service", service, "--traces", fmt.Sprint(numTraces)})

	var traces []*aggregator.TracePayload
	var err error
	s.T().Log("Waiting for traces")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		traces, err = s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
	}, 2*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), traces)
	trace := traces[0]
	assert.Equal(s.T(), "none", trace.Env)
	require.NotEmpty(s.T(), trace.TracerPayloads)
	tp := trace.TracerPayloads[0]
	require.NotEmpty(s.T(), tp.Chunks)
	require.NotEmpty(s.T(), tp.Chunks[0].Spans)
	spans := tp.Chunks[0].Spans
	for _, sp := range spans {
		assert.Equal(s.T(), service, sp.Service)
		assert.Equal(s.T(), "telemetrygen", sp.Meta["otel.library.name"])
	}
}

func (s *linuxTestSuite) TestOTLPMetrics() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	serviceAttribute := fmt.Sprintf("service.name=\"%v\"", service)
	numMetrics := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "metrics", []string{"--metrics", fmt.Sprint(numMetrics), "--otlp-attributes", serviceAttribute})

	s.T().Log("Waiting for metrics")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		serviceTag := "service:" + service
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("gen", fakeintake.WithTags[*aggregator.MetricSeries]([]string{serviceTag}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) TestOTLPLogs() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	serviceAttribute := fmt.Sprintf("service.name=\"%v\"", service)
	numLogs := 10
	logBody := "telemetrygen log"

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "logs", []string{"--logs", fmt.Sprint(numLogs), "--otlp-attributes", serviceAttribute, "--body", logBody})

	var logs []*aggregator.Log
	var err error
	s.T().Log("Waiting for logs")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err = s.Env().FakeIntake.Client().FilterLogs(service)
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 2*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), logs)
	for _, log := range logs {
		assert.Contains(s.T(), log.Message, logBody)
	}
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
							Name:    "telemetrygen-job",
							Image:   "ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:latest",
							Command: append([]string{"/telemetrygen", telemetry, "--otlp-endpoint", otlpEndpoint, "--otlp-insecure"}, options...),
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
