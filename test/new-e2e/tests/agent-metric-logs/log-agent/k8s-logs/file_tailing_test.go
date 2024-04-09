// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"context"
	"io"
	"os"
	"slices"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	customkind "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/customkind"
)

type myKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyKindSuite(t *testing.T) {
	e2e.Run(t, &myKindSuite{}, e2e.WithProvisioner(customkind.Provisioner()))
}

func (v *myKindSuite) TestSingleLog() {
	var backOffLimit int32 = 4
	testLogMessage := "Test log message"

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
	assert.NoError(v.T(), err, "Could not properly start job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")
		assert.Contains(c, logsServiceNames, "ubuntu", "Ubuntu service not found")

		if slices.Contains(logsServiceNames, "ubuntu") {
			filteredLogs, err := v.Env().FakeIntake.Client().FilterLogs("ubuntu")
			assert.NoError(c, err, "Error filtering logs")
			assert.Equal(c, filteredLogs[0].Message, testLogMessage, "Test log doesn't match")

			// Check container metatdata
			assert.Equal(c, filteredLogs[0].Service, "ubuntu", "Could not find service")
			assert.NotNil(c, filteredLogs[0].HostName, "Hostname not found")
			assert.NotNil(c, filteredLogs[0].Tags, "Log tags not found")
		}

	}, 1*time.Minute, 10*time.Second)
}

func (v *myKindSuite) TestCorruptedLog() {
	var backOffLimit int32 = 4
	file, err := os.Open("long_line_log.txt")
	assert.NoError(v.T(), err, "Could not open long line file.")
	io.ReadAll(file)

	b, err := io.ReadAll(file)

	defer func() {
		err := file.Close()
		assert.NoError(v.T(), err, "Could not close file.")
	}()

	jobSpcec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "long-line-job",
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "long-line-job",
							Image:   "ubuntu",
							Command: []string{"echo", string(b)},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err = v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	assert.NoError(v.T(), err, "Could not properly start job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")
		assert.Contains(c, logsServiceNames, "ubuntu", "Ubuntu service not found")

		if slices.Contains(logsServiceNames, "ubuntu") {
			filteredLogs, err := v.Env().FakeIntake.Client().FilterLogs("ubuntu")
			assert.NoError(c, err, "Error filtering logs")
			assert.Equal(c, filteredLogs[0].Message, string(b), "Test log doesn't match")
		}

	}, 2*time.Minute, 10*time.Second)
}
