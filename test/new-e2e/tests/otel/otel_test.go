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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local/kubernetes"
)

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed collector.yml
var collectorConfig string

func TestOtel(t *testing.T) {
	fmt.Println("config", collectorConfig)
	e2e.Run(t, &linuxTestSuite{}, e2e.WithProvisioner(localkubernetes.Provisioner(localkubernetes.WithAgentOptions(kubernetesagentparams.WithOTELAgent(), kubernetesagentparams.WithOTELConfig(collectorConfig)))))
}

func (s *linuxTestSuite) TestOtelAgentInstalled() {
	res, _ := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.TODO(), v1.ListOptions{})
	containsOtelAgent := false
	for _, pod := range res.Items {
		if strings.Contains(pod.Name, "otel-agent") {
			containsOtelAgent = true
			break
		}
	}
	assert.True(s.T(), containsOtelAgent, "Otel Agent not found")
	assert.Equal(s.T(), s.Env().Agent.NodeAgent, "otel-agent")
}

func (s *linuxTestSuite) TestOTelPipelines() {
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	var ttlSecondsAfterFinished int32 = 300
	var backOffLimit int32 = 4

	var otlpEndpoint string
	res, _ := s.Env().KubernetesCluster.Client().CoreV1().Endpoints("datadog").List(context.TODO(), v1.ListOptions{})
	for _, item := range res.Items {
		if strings.HasPrefix(item.Name, "dda-linux-datadog") && !strings.Contains(item.Name, "cluster") {
			otlpEndpoint = fmt.Sprintf("%v:4317", item.Name)
		}
	}
	assert.False(s.T(), otlpEndpoint == "", "Failed to get OTel Agent endpoint")

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "telemetrygen-job",
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
							Command: []string{"/telemetrygen", "traces", "--otlp-endpoint", otlpEndpoint, "--otlp-insecure", "--traces", "20", "--service", "telemetrygen-job"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := s.Env().KubernetesCluster.Client().BatchV1().Jobs("datadog").Create(context.TODO(), jobSpec, metav1.CreateOptions{})
	assert.NoError(s.T(), err, "Could not properly start job")

	time.Sleep(5 * time.Minute)

	s.EventuallyWithT(func(c *assert.CollectT) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(c, err, "Error starting job")
		fmt.Println(traces)
	}, 1*time.Minute, 10*time.Second)

}
