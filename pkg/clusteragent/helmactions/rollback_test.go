// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func validOptions() RollbackOptions {
	return RollbackOptions{
		Release:            "myrel",
		ReleaseNamespace:   "prod",
		JobNamespace:       "ops",
		ServiceAccountName: "helm-sa",
	}
}

func TestRollbackOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*RollbackOptions)
		wantErr bool
	}{
		{"valid", func(*RollbackOptions) {}, false},
		{"missing release", func(o *RollbackOptions) { o.Release = "" }, true},
		{"missing release namespace", func(o *RollbackOptions) { o.ReleaseNamespace = "" }, true},
		{"missing job namespace", func(o *RollbackOptions) { o.JobNamespace = "" }, true},
		{"missing service account", func(o *RollbackOptions) { o.ServiceAccountName = "" }, true},
		{"negative revision", func(o *RollbackOptions) { o.Revision = -1 }, true},
		{"zero revision is allowed", func(o *RollbackOptions) { o.Revision = 0 }, false},
		{"positive revision is allowed", func(o *RollbackOptions) { o.Revision = 7 }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := validOptions()
			tt.mutate(&o)
			err := o.validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildRollbackJob_Defaults(t *testing.T) {
	job := buildRollbackJob(validOptions())

	assert.Equal(t, "ops", job.Namespace)
	assert.Equal(t, rollbackJobNamePrefix, job.GenerateName)
	assert.Equal(t, "myrel", job.Labels[labelRelease])
	assert.Equal(t, "prod", job.Labels[labelNamespace])

	require.NotNil(t, job.Spec.BackoffLimit)
	assert.Equal(t, int32(0), *job.Spec.BackoffLimit)
	require.NotNil(t, job.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, defaultTTLSecondsAfterFinished, *job.Spec.TTLSecondsAfterFinished)

	pod := job.Spec.Template.Spec
	assert.Equal(t, "helm-sa", pod.ServiceAccountName)
	assert.Equal(t, corev1.RestartPolicyNever, pod.RestartPolicy)
	require.Len(t, pod.Containers, 1)
	c := pod.Containers[0]
	assert.Equal(t, helmContainerName, c.Name)
	assert.Equal(t, DefaultHelmImage, c.Image)
	assert.Equal(t, []string{"helm"}, c.Command)
	assert.Equal(t, []string{"rollback", "myrel", "--namespace", "prod"}, c.Args)
}

func TestBuildRollbackJob_ExplicitRevision(t *testing.T) {
	opts := validOptions()
	opts.Revision = 5
	job := buildRollbackJob(opts)
	assert.Equal(t, []string{"rollback", "myrel", "5", "--namespace", "prod"}, job.Spec.Template.Spec.Containers[0].Args)
}

func TestBuildRollbackJob_Driver(t *testing.T) {
	t.Run("unset omits HELM_DRIVER", func(t *testing.T) {
		job := buildRollbackJob(validOptions())
		assert.Empty(t, job.Spec.Template.Spec.Containers[0].Env)
	})

	t.Run("set propagates as env var", func(t *testing.T) {
		opts := validOptions()
		opts.Driver = "configmap"
		job := buildRollbackJob(opts)
		env := job.Spec.Template.Spec.Containers[0].Env
		require.Len(t, env, 1)
		assert.Equal(t, "HELM_DRIVER", env[0].Name)
		assert.Equal(t, "configmap", env[0].Value)
	})
}

func TestBuildRollbackJob_Overrides(t *testing.T) {
	backoff := int32(3)
	ttl := int32(60)
	opts := validOptions()
	opts.Image = "myrepo/helm:3.14"
	opts.BackoffLimit = &backoff
	opts.TTLSecondsAfterFinished = &ttl
	opts.ExtraLabels = map[string]string{"team": "platform", labelComponent: "ignored-because-overwrite"}

	job := buildRollbackJob(opts)

	assert.Equal(t, "myrepo/helm:3.14", job.Spec.Template.Spec.Containers[0].Image)
	require.NotNil(t, job.Spec.BackoffLimit)
	assert.Equal(t, int32(3), *job.Spec.BackoffLimit)
	require.NotNil(t, job.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, int32(60), *job.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, "platform", job.Labels["team"])
	// ExtraLabels can override package-set labels.
	assert.Equal(t, "ignored-because-overwrite", job.Labels[labelComponent])
	// Labels are mirrored onto the pod template.
	assert.Equal(t, job.Labels, job.Spec.Template.Labels)
}

func TestRollbackExecutor_Run_CreatesJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewRollbackExecutor(clientset)

	opts := validOptions()
	opts.Revision = 3
	created, err := executor.Run(context.Background(), opts)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "ops", created.Namespace)

	jobs, err := clientset.BatchV1().Jobs("ops").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, jobs.Items, 1)
	assert.Equal(t, []string{"rollback", "myrel", "3", "--namespace", "prod"},
		jobs.Items[0].Spec.Template.Spec.Containers[0].Args)
}

func TestRollbackExecutor_Run_ValidationError(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewRollbackExecutor(clientset)

	_, err := executor.Run(context.Background(), RollbackOptions{})
	assert.Error(t, err)

	jobs, listErr := clientset.BatchV1().Jobs("").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, listErr)
	assert.Empty(t, jobs.Items, "no job should be created on validation failure")
}
