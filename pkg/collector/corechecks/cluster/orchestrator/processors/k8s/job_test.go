// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestJobHandlers_ExtractResource(t *testing.T) {
	handlers := &JobHandlers{}

	// Create test job
	job := createTestJob("test-job", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Extract resource
	resourceModel := handlers.ExtractResource(ctx, job)

	// Validate extraction
	jobModel, ok := resourceModel.(*model.Job)
	assert.True(t, ok)
	assert.NotNil(t, jobModel)
	assert.Equal(t, "test-job", jobModel.Metadata.Name)
	assert.Equal(t, "test-namespace", jobModel.Metadata.Namespace)
	assert.NotNil(t, jobModel.Spec)
	assert.NotNil(t, jobModel.Status)
}

func TestJobHandlers_ResourceList(t *testing.T) {
	handlers := &JobHandlers{}

	// Create test jobs
	job1 := createTestJob("job-1", "namespace-1")
	job2 := createTestJob("job-2", "namespace-2")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Convert list
	resourceList := []*batchv1.Job{job1, job2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*batchv1.Job)
	assert.True(t, ok)
	assert.Equal(t, "job-1", resource1.Name)
	assert.NotSame(t, job1, resource1) // Should be a copy

	resource2, ok := resources[1].(*batchv1.Job)
	assert.True(t, ok)
	assert.Equal(t, "job-2", resource2.Name)
	assert.NotSame(t, job2, resource2) // Should be a copy
}

func TestJobHandlers_ResourceUID(t *testing.T) {
	handlers := &JobHandlers{}

	job := createTestJob("test-job", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	job.UID = expectedUID

	uid := handlers.ResourceUID(nil, job)
	assert.Equal(t, expectedUID, uid)
}

func TestJobHandlers_ResourceVersion(t *testing.T) {
	handlers := &JobHandlers{}

	job := createTestJob("test-job", "test-namespace")
	expectedVersion := "v123"
	job.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.Job{}

	version := handlers.ResourceVersion(nil, job, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestJobHandlers_BuildMessageBody(t *testing.T) {
	handlers := &JobHandlers{}

	job1 := createTestJob("job-1", "namespace-1")
	job2 := createTestJob("job-2", "namespace-2")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	job1Model := k8sTransformers.ExtractJob(ctx, job1)
	job2Model := k8sTransformers.ExtractJob(ctx, job2)

	// Build message body
	resourceModels := []interface{}{job1Model, job2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorJob)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Jobs, 2)
	assert.Equal(t, "job-1", collectorMsg.Jobs[0].Metadata.Name)
	assert.Equal(t, "job-2", collectorMsg.Jobs[1].Metadata.Name)
}

func TestJobHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &JobHandlers{}

	job := createTestJob("test-job", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Job",
			APIVersion:       "batch/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.Job{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, job, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "Job", job.Kind)
	assert.Equal(t, "batch/v1", job.APIVersion)
}

func TestJobHandlers_AfterMarshalling(t *testing.T) {
	handlers := &JobHandlers{}

	job := createTestJob("test-job", "test-namespace")
	resourceModel := &model.Job{}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Test YAML
	testYAML := []byte(`{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, job, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestJobHandlers_GetMetadataTags(t *testing.T) {
	handlers := &JobHandlers{}

	// Create job model with tags
	jobModel := &model.Job{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, jobModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestJobHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &JobHandlers{}

	// Create job with sensitive annotations and labels
	job := createTestJob("test-job", "test-namespace")
	job.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	job.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, job)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", job.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", job.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestJobProcessor_Process(t *testing.T) {
	// Create test jobs with unique UIDs
	job1 := createTestJob("job-1", "namespace-1")
	job1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	job1.ResourceVersion = "1210"

	job2 := createTestJob("job-2", "namespace-2")
	job2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	job2.ResourceVersion = "1310"

	// Create fake client
	client := fake.NewClientset(job1, job2)
	apiClient := &apiserver.APIClient{Cl: client}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			NodeType:         orchestrator.K8sJob,
			Kind:             "Job",
			APIVersion:       "batch/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process jobs
	processor := processors.NewProcessor(&JobHandlers{})
	result, listed, processed := processor.Process(ctx, []*batchv1.Job{job1, job2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorJob)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Jobs, 2)

	expectedJob1 := k8sTransformers.ExtractJob(ctx, job1)

	assert.Equal(t, expectedJob1.Metadata, metaMsg.Jobs[0].Metadata)
	assert.Equal(t, expectedJob1.Spec, metaMsg.Jobs[0].Spec)
	assert.Equal(t, expectedJob1.Status, metaMsg.Jobs[0].Status)
	assert.Equal(t, expectedJob1.Conditions, metaMsg.Jobs[0].Conditions)
	assert.Equal(t, expectedJob1.Tags, metaMsg.Jobs[0].Tags)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.GroupId)
	assert.Equal(t, "test-host", manifestMsg.HostName)
	assert.Len(t, manifestMsg.Manifests, 2)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)

	// Validate manifest details
	manifest1 := manifestMsg.Manifests[0]
	assert.Equal(t, job1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, job1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(6), manifest1.Type)
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestJob batchv1.Job
	err := json.Unmarshal(manifest1.Content, &actualManifestJob)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestJob.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestJob.ObjectMeta.CreationTimestamp.Time.UTC()}
	if actualManifestJob.Status.StartTime != nil {
		actualManifestJob.Status.StartTime = &metav1.Time{Time: actualManifestJob.Status.StartTime.Time.UTC()}
	}
	if actualManifestJob.Status.CompletionTime != nil {
		actualManifestJob.Status.CompletionTime = &metav1.Time{Time: actualManifestJob.Status.CompletionTime.Time.UTC()}
	}
	actualManifestJob.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestJob.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	actualManifestJob.Status.Conditions[0].LastProbeTime = metav1.Time{Time: actualManifestJob.Status.Conditions[0].LastProbeTime.Time.UTC()}
	actualManifestJob.Status.StartTime = &metav1.Time{Time: actualManifestJob.Status.StartTime.Time.UTC()}
	assert.Equal(t, job1.ObjectMeta, actualManifestJob.ObjectMeta)
	assert.Equal(t, job1.Spec, actualManifestJob.Spec)
	assert.Equal(t, job1.Status, actualManifestJob.Status)
}

func createTestJob(name, namespace string) *batchv1.Job {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	startTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))
	completionTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 35, 0, 0, time.UTC))
	lastTransitionTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 32, 0, 0, time.UTC))
	lastProbeTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 33, 0, 0, time.UTC))

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			ResourceVersion:   "1210",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1beta1",
					Controller: pointer.Ptr(true),
					Kind:       "CronJob",
					Name:       "test-cronjob",
					UID:        "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
				},
			},
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: pointer.Ptr(int64(300)),
			BackoffLimit:          pointer.Ptr(int32(6)),
			Completions:           pointer.Ptr(int32(1)),
			Parallelism:           pointer.Ptr(int32(1)),
			ManualSelector:        pointer.Ptr(false),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"my-app"},
					},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
						"job-name":       name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
		Status: batchv1.JobStatus{
			Active:         1,
			Succeeded:      0,
			Failed:         0,
			StartTime:      &startTime,
			CompletionTime: &completionTime,
			UncountedTerminatedPods: &batchv1.UncountedTerminatedPods{
				Succeeded: []types.UID{"pod-1", "pod-2"},
				Failed:    []types.UID{"pod-3"},
			},
			Conditions: []batchv1.JobCondition{
				{
					Type:               batchv1.JobComplete,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: lastTransitionTime,
					LastProbeTime:      lastProbeTime,
					Reason:             "JobCompleted",
					Message:            "Job completed successfully",
				},
			},
		},
	}
}
