// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"context"
	_ "embed"
	"fmt"
	kindfilelogger "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/kindfilelogging"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type k8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestK8sSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &k8sSuite{}, e2e.WithProvisioner(kindfilelogger.Provisioner()))
}

func (v *k8sSuite) TestSingleLogAndMetadata() {
	err := v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(v.T(), err, "Could not reset the Fake Intake")
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

	_, err = v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	require.NoError(v.T(), err, "Could not properly start job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")
		if err != nil {
			return
		}

		if assert.Contains(c, logsServiceNames, "ubuntu", "Ubuntu service not found") {
			filteredLogs, err := v.Env().FakeIntake.Client().FilterLogs("ubuntu")
			assert.NoError(c, err, "Error filtering logs")
			if err != nil {
				return
			}
			if assert.NotEmpty(c, filteredLogs, "Fake Intake returned no logs even though log service name exists") {
				assert.Equal(c, testLogMessage, filteredLogs[0].Message, "Test log doesn't match")
				// Check container metatdata
				assert.Equal(c, filteredLogs[0].Service, "ubuntu", "Could not find service")
				assert.NotNil(c, filteredLogs[0].HostName, "Hostname not found")
				assert.NotNil(c, filteredLogs[0].Tags, "Log tags not found")
			}
		}

	}, 1*time.Minute, 10*time.Second)
}

//go:embed long_line_log.txt
var longLineLog string

func (v *k8sSuite) TestLongLogLine() {
	err := v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(v.T(), err, "Could not reset the FakeIntake")
	var backOffLimit int32 = 4

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
							Command: []string{"echo", longLineLog},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err = v.Env().KubernetesCluster.Client().BatchV1().Jobs("default").Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	require.NoError(v.T(), err, "Could not properly start job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")
		if err != nil {
			return
		}

		if assert.Contains(c, logsServiceNames, "ubuntu", "Ubuntu service not found") {
			filteredLogs, err := v.Env().FakeIntake.Client().FilterLogs("ubuntu")
			assert.NoError(c, err, "Error filtering logs")
			if err != nil {
				return
			}
			if assert.NotEmpty(c, filteredLogs, "Fake Intake returned no logs even though log service name exists") {
				assert.Equal(c, longLineLog, fmt.Sprintf("%s%s", filteredLogs[0].Message, "\n"), "Test log doesn't match")
			}
		}

	}, 1*time.Minute, 10*time.Second)
}

func (v *k8sSuite) TestContainerExclude() {
	err := v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(v.T(), err, "Could not reset the Fake Intake")

	// We're testing exclusion via namespace, so we have to create a new namespace
	namespaceName := "exclude-namespace"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	_, err = v.Env().KubernetesCluster.Client().CoreV1().Namespaces().Create(context.TODO(), namespace, metav1.CreateOptions{})
	require.NoError(v.T(), err, "Could not create namespace")

	var backOffLimit int32 = 4
	testLogMessage := "Test log message here"

	jobSpcec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "exclude-job",
			Namespace: namespaceName,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "exclude-job",
							Image:   "alpine",
							Command: []string{"echo", testLogMessage},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: &backOffLimit,
		},
	}

	_, err = v.Env().KubernetesCluster.Client().BatchV1().Jobs(namespaceName).Create(context.TODO(), jobSpcec, metav1.CreateOptions{})
	require.NoError(v.T(), err, "Could not properly start job")

	v.EventuallyWithT(func(c *assert.CollectT) {
		logsServiceNames, err := v.Env().FakeIntake.Client().GetLogServiceNames()
		assert.NoError(c, err, "Error starting job")
		if err != nil {
			return
		}
		assert.NotContains(c, logsServiceNames, "alpine", "Alpine service found after excluded")
	}, 1*time.Minute, 10*time.Second)
}
