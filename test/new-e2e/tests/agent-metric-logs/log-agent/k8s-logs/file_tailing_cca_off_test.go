// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	kindfilelogger "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/kindfilelogging"
)

type k8sCCAOffSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sCCAOff(t *testing.T) {
	e2e.Run(t, &k8sCCAOffSuite{}, e2e.WithProvisioner(kindfilelogger.Provisioner(kindfilelogger.WithAgentOptions(kubernetesagentparams.WithoutLogsContainerCollectAll()))))
}

func (v *k8sCCAOffSuite) TestADAnnotations() {
	v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	var backOffLimit int32 = 4
	testLogMessage := "Annotations pod"

	jobSpcec := &batchv1.Job{
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
							Name:    "annotations-job",
							Image:   "ubuntu",
							Command: []string{"echo", testLogMessage},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	assert.NoError(v.T(), err, "Could not start autodiscovery job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")

		if assert.Contains(c, logsServiceNames, "ubuntu", "Ubuntu service not found") {
			filteredLogs, err := v.Env().FakeIntake.Client().FilterLogs("ubuntu")
			assert.NoError(c, err, "Error filtering logs")
			assert.Equal(c, testLogMessage, filteredLogs[0].Message, "Test log doesn't match")
		}
	}, 1*time.Minute, 10*time.Second)
}

func (v *k8sCCAOffSuite) TestCCAOff() {
	v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	var backOffLimit int32 = 4
	testLogMessage := "Test pod"

	jobSpcec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cca-off-job",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "cca-off-job",
							Image:   "ubuntu",
							Command: []string{"echo", testLogMessage},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	assert.NoError(v.T(), err, "Could not start job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")
		assert.NotContains(c, logsServiceNames, "ubuntu", "Ubuntu service found with container collect all off")
	}, 1*time.Minute, 10*time.Second)
}
