// Unless explicitly stated otherwise all files in this repository are licensed // under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"context"
	"fmt"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	customkind "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/customkind"
)

type myKindSuite2 struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyKindSuite2(t *testing.T) {
	helmValues := `
    datadog:
      kubelet:
        tlsVerify: false
      clusterName: "random-cluster-name"
      envDict:
        DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL: "true"
    agents:
      useHostNetwork: true
      `

	e2e.Run(t, &myKindSuite2{}, e2e.WithProvisioner(customkind.Provisioner(customkind.WithAgentOptions(kubernetesagentparams.WithHelmValues(helmValues)))))
}

func (v *myKindSuite2) TestADAnnotations() {
	v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	var backOffLimit int32 = 4
	testLogMessage := "Annotations pod"

	jobSpcec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "annotations-job",
			Namespace: "default",
			Annotations: map[string]string{
				"ad.datadoghq.com/annotations-job.logs": "[{\"source\": \"test-container\"}]",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
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
		fmt.Println(logsServiceNames)
		assert.NoError(c, err, "Error starting job")
		assert.Contains(c, logsServiceNames, "ubuntu", "Ubuntu service not found")
	}, 1*time.Minute, 10*time.Second)
}
