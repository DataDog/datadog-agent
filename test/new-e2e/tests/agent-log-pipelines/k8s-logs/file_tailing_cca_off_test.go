// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"context"
	"slices"
	"testing"

	apps "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	kindfilelogger "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/kindfilelogging"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	k8sutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/k8s"
)

type k8sCCAOffSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sCCAOff(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &k8sCCAOffSuite{}, e2e.WithProvisioner(kindfilelogger.Provisioner(kindfilelogger.WithAgentOptions(kubernetesagentparams.WithoutLogsContainerCollectAll()))))
}

func (v *k8sCCAOffSuite) TestADAnnotations() {
	err := v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(v.T(), err, "Could not reset the Fake Intake")
	var backOffLimit int32 = 4
	testLogMessage := "Annotations pod"

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "annotations-job",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/annotations-job.logs": "[{\"source\": \"test-container\"}]",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "annotations-job",
							Image: "ghcr.io/datadog/apps-alpine:" + apps.Version,
							// Sleep is added here so k8s doesn't kill the container before
							// the agent container can detect it.
							Command: []string{"sh", "-c", "echo '" + testLogMessage + "' && sleep 10"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err = v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpec, metav1.CreateOptions{})
	require.NoError(v.T(), err, "Could not create autodiscovery job")

	_, err = k8sutils.WaitForJobPodRunning(context.TODO(), v.Env().KubernetesCluster.Client(), "default", "annotations-job", jobPodStartTimeout)
	if err != nil {
		require.Fail(v.T(), "Annotations job pod failed to start",
			"%v\n%s", err, k8sutils.DescribeJob(context.TODO(), v.Env().KubernetesCluster.Client(), "default", "annotations-job"))
	}

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		if !assert.NoError(c, err, "Error getting log service names") {
			return
		}

		if !slices.Contains(logsServiceNames, "apps-alpine") {
			assert.Fail(c, "Alpine service not found",
				"Known services: %q\n%s",
				logsServiceNames, fakeintakeRouteStats(v.Env().FakeIntake))
			return
		}

		filteredLogs, err := v.Env().FakeIntake.Client().FilterLogs("apps-alpine")
		if !assert.NoError(c, err, "Error filtering logs") {
			return
		}
		if assert.NotEmpty(c, filteredLogs, "Fake Intake returned no logs even though log service name exists") {
			assert.Equal(c, testLogMessage, filteredLogs[0].Message, "Test log doesn't match")
		}
	}, 1*time.Minute, 10*time.Second)
}

func (v *k8sCCAOffSuite) TestCCAOff() {
	err := v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(v.T(), err, "Could not reset the Fake Intake")
	var backOffLimit int32 = 4
	testLogMessage := "Test pod"

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cca-off-job",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "cca-off-job",
							Image: "ghcr.io/datadog/apps-alpine:" + apps.Version,
							// Sleep is added here so k8s doesn't kill the container before
							// the agent container can detect it.
							Command: []string{"sh", "-c", "echo '" + testLogMessage + "' && sleep 10"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err = v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpec, metav1.CreateOptions{})
	require.NoError(v.T(), err, "Could not create CCA-off job")

	_, err = k8sutils.WaitForJobPodRunning(context.TODO(), v.Env().KubernetesCluster.Client(), "default", "cca-off-job", jobPodStartTimeout)
	if err != nil {
		require.Fail(v.T(), "CCA-off job pod failed to start",
			"%v\n%s", err, k8sutils.DescribeJob(context.TODO(), v.Env().KubernetesCluster.Client(), "default", "cca-off-job"))
	}

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		if !assert.NoError(c, err, "Error getting log service names") {
			return
		}
		assert.NotContains(c, logsServiceNames, "apps-alpine", "Alpine service found with container collect all off")
	}, 1*time.Minute, 10*time.Second)
}

func (v *k8sCCAOffSuite) AfterTest(suiteName, testName string) {
	if v.T().Failed() {
		v.T().Log(fakeintakeRouteStats(v.Env().FakeIntake))
	}
	v.BaseSuite.AfterTest(suiteName, testName)
}
