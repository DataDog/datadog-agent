// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"context"
	"fmt"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	customkind "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/customkind"

	// "github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
)

type myKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyKindSuite(t *testing.T) {
	e2e.Run(t, &myKindSuite{}, e2e.WithProvisioner(customkind.Provisioner()))
}

func (v *myKindSuite) TestAgentOutput() {

	var backOffLimit int32 = 4

	jobSpcec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-0",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "query-job-0",
							Image:   "ubunutu",
							Command: []string{"echo", "'a thing'"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	assert.NoError(v.T(), err, "Could not properly start job")

	// logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
	// fmt.Printf("logs service names: %v", logsServiceNames)

	// logs, err := v.Env().FakeIntake.Client().FilterLogs("query-job", fi.WithMessageContaining("hello world"))
	// assert.NoError(v.T(), err, "Failed to filter logs")
	// fmt.Println(logs)

	// fmt.Printf("Fake intake output : %s\n", fakeIntakeOutput.URL)
	// if v.Env().FakeIntake == nil {
	// 	fmt.Println("fake intake is nil")
	// } else {
	// 	fmt.Println("fake intake is not nil")
	// }
}

func (v *myKindSuite) TestBatchJob() {
	var backOffLimit int32 = 4

	jobSpcec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-1",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "query-job-1",
							Image:   "ubunutu",
							Command: []string{"echo", "Another thing \n"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err := v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	assert.NoError(v.T(), err, "Could not properly start job")

	jobList, err := v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").List(context.TODO(), metav1.ListOptions{})
	fmt.Printf("job list: %s", jobList.String())

	logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
	logsConnections, err := v.Env().FakeIntake.Client().GetConnectionsNames()
	fmt.Printf("logs service names: %v\n", logsServiceNames)
	fmt.Printf("logs connections names: %v\n", logsConnections)
}
