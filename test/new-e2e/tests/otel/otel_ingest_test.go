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
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

type otelIngestTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelIngest(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
`
	t.Parallel()
	e2e.Run(t, &otelIngestTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otelIngestTestSuite) TestOTLPTraces() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	numTraces := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "traces", []string{"--service", service, "--traces", fmt.Sprint(numTraces)})

	s.T().Log("Waiting for traces")
	s.EventuallyWithT(func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err)
		assert.NotEmpty(c, traces)
		trace := traces[0]
		assert.Equal(c, "none", trace.Env)
		assert.NotEmpty(c, trace.TracerPayloads)
		tp := trace.TracerPayloads[0]
		assert.NotEmpty(c, tp.Chunks)
		assert.NotEmpty(c, tp.Chunks[0].Spans)
		spans := tp.Chunks[0].Spans
		for _, sp := range spans {
			assert.Equal(c, service, sp.Service)
			assert.Equal(c, "telemetrygen", sp.Meta["otel.library.name"])
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *otelIngestTestSuite) TestOTLPMetrics() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	serviceAttribute := fmt.Sprintf("service.name=\"%v\"", service)
	numMetrics := 10

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "metrics", []string{"--metrics", fmt.Sprint(numMetrics), "--otlp-attributes", serviceAttribute})

	s.T().Log("Waiting for metrics")
	s.EventuallyWithT(func(c *assert.CollectT) {
		serviceTag := "service:" + service
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("gen", fakeintake.WithTags[*aggregator.MetricSeries]([]string{serviceTag}))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 2*time.Minute, 10*time.Second)
}

func (s *otelIngestTestSuite) TestOTLPLogs() {
	ctx := context.Background()
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	service := "telemetrygen-job"
	serviceAttribute := fmt.Sprintf("service.name=\"%v\"", service)
	numLogs := 10
	logBody := "telemetrygen log"

	s.T().Log("Starting telemetrygen")
	s.createTelemetrygenJob(ctx, "logs", []string{"--logs", fmt.Sprint(numLogs), "--otlp-attributes", serviceAttribute, "--body", logBody})

	s.T().Log("Waiting for logs")
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(service)
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
		for _, log := range logs {
			assert.Contains(c, log.Message, logBody)
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *otelIngestTestSuite) createTelemetrygenJob(ctx context.Context, telemetry string, options []string) {
	var ttlSecondsAfterFinished int32 = 600 //nolint:revive // We want to see this is explicitly set to 0
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
	assert.NoError(s.T(), err, "Could not properly start job")
}
