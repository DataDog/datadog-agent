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

func TestCronJobV1Handlers_ExtractResource(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	// Create test cron job
	cronJob := createTestCronJobV1("test-cronjob", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, cronJob)

	// Validate extraction
	cronJobModel, ok := resourceModel.(*model.CronJob)
	assert.True(t, ok)
	assert.NotNil(t, cronJobModel)
	assert.Equal(t, "test-cronjob", cronJobModel.Metadata.Name)
	assert.Equal(t, "test-namespace", cronJobModel.Metadata.Namespace)
	assert.Equal(t, "*/5 * * * *", cronJobModel.Spec.Schedule)
	assert.Equal(t, "Forbid", cronJobModel.Spec.ConcurrencyPolicy)
}

func TestCronJobV1Handlers_ResourceList(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	// Create test cron jobs
	cronJob1 := createTestCronJobV1("cronjob-1", "namespace-1")
	cronJob2 := createTestCronJobV1("cronjob-2", "namespace-2")

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
	resourceList := []*batchv1.CronJob{cronJob1, cronJob2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*batchv1.CronJob)
	assert.True(t, ok)
	assert.Equal(t, "cronjob-1", resource1.Name)
	assert.NotSame(t, cronJob1, resource1) // Should be a copy

	resource2, ok := resources[1].(*batchv1.CronJob)
	assert.True(t, ok)
	assert.Equal(t, "cronjob-2", resource2.Name)
	assert.NotSame(t, cronJob2, resource2) // Should be a copy
}

func TestCronJobV1Handlers_ResourceUID(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	cronJob := createTestCronJobV1("test-cronjob", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	cronJob.UID = expectedUID

	uid := handlers.ResourceUID(nil, cronJob)
	assert.Equal(t, expectedUID, uid)
}

func TestCronJobV1Handlers_ResourceVersion(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	cronJob := createTestCronJobV1("test-cronjob", "test-namespace")
	expectedVersion := "v123"
	cronJob.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.CronJob{}

	version := handlers.ResourceVersion(nil, cronJob, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestCronJobV1Handlers_BuildMessageBody(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	cronJob1 := createTestCronJobV1("cronjob-1", "namespace-1")
	cronJob2 := createTestCronJobV1("cronjob-2", "namespace-2")

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

	cronJob1Model := k8sTransformers.ExtractCronJobV1(ctx, cronJob1)
	cronJob2Model := k8sTransformers.ExtractCronJobV1(ctx, cronJob2)

	// Build message body
	resourceModels := []interface{}{cronJob1Model, cronJob2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorCronJob)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.CronJobs, 2)
	assert.Equal(t, "cronjob-1", collectorMsg.CronJobs[0].Metadata.Name)
	assert.Equal(t, "cronjob-2", collectorMsg.CronJobs[1].Metadata.Name)
}

func TestCronJobV1Handlers_BeforeMarshalling(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	cronJob := createTestCronJobV1("test-cronjob", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "CronJob",
			APIVersion:       "batch/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.CronJob{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, cronJob, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "CronJob", cronJob.Kind)
	assert.Equal(t, "batch/v1", cronJob.APIVersion)
}

func TestCronJobV1Handlers_AfterMarshalling(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	cronJob := createTestCronJobV1("test-cronjob", "test-namespace")
	resourceModel := &model.CronJob{}

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
	testYAML := []byte(`{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, cronJob, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestCronJobV1Handlers_GetMetadataTags(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	// Create cron job model with tags
	cronJobModel := &model.CronJob{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, cronJobModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestCronJobV1Handlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &CronJobV1Handlers{}

	// Create cron job with sensitive annotations and labels
	cronJob := createTestCronJobV1("test-cronjob", "test-namespace")
	cronJob.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	cronJob.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, cronJob)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", cronJob.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", cronJob.Labels["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "my-annotation", cronJob.Annotations["annotation"])
	assert.Equal(t, "my-app", cronJob.Labels["app"])
}

func TestCronJobV1Processor_Process(t *testing.T) {
	// Create test cron jobs with unique UIDs
	cronJob1 := createTestCronJobV1("cronjob-1", "namespace-1")
	cronJob1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	cronJob1.ResourceVersion = "1204"

	cronJob2 := createTestCronJobV1("cronjob-2", "namespace-2")
	cronJob2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	cronJob2.ResourceVersion = "1304"

	// Create fake client
	client := fake.NewClientset(cronJob1, cronJob2)
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
			NodeType:         orchestrator.K8sCronJob,
			Kind:             "CronJob",
			APIVersion:       "batch/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process cron jobs
	processor := processors.NewProcessor(&CronJobV1Handlers{})
	result, listed, processed := processor.Process(ctx, []*batchv1.CronJob{cronJob1, cronJob2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorCronJob)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.CronJobs, 2)

	expectedCronJob1 := k8sTransformers.ExtractCronJobV1(ctx, cronJob1)

	assert.Equal(t, expectedCronJob1.Metadata, metaMsg.CronJobs[0].Metadata)
	assert.Equal(t, expectedCronJob1.Spec, metaMsg.CronJobs[0].Spec)
	assert.Equal(t, expectedCronJob1.Status, metaMsg.CronJobs[0].Status)
	assert.Equal(t, expectedCronJob1.Tags, metaMsg.CronJobs[0].Tags)

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
	assert.Equal(t, cronJob1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, cronJob1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(7), manifest1.Type) // K8sCronJob
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestCronJob batchv1.CronJob
	err := json.Unmarshal(manifest1.Content, &actualManifestCronJob)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestCronJob.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestCronJob.ObjectMeta.CreationTimestamp.Time.UTC()}
	if actualManifestCronJob.Status.LastScheduleTime != nil {
		actualManifestCronJob.Status.LastScheduleTime = &metav1.Time{Time: actualManifestCronJob.Status.LastScheduleTime.Time.UTC()}
	}
	if actualManifestCronJob.Status.LastSuccessfulTime != nil {
		actualManifestCronJob.Status.LastSuccessfulTime = &metav1.Time{Time: actualManifestCronJob.Status.LastSuccessfulTime.Time.UTC()}
	}
	assert.Equal(t, cronJob1.ObjectMeta, actualManifestCronJob.ObjectMeta)
	assert.Equal(t, cronJob1.Spec, actualManifestCronJob.Spec)
	assert.Equal(t, cronJob1.Status, actualManifestCronJob.Status)
}

func createTestCronJobV1(name, namespace string) *batchv1.CronJob {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	lastScheduleTime := metav1.NewTime(time.Date(2021, time.April, 17, 14, 30, 0, 0, time.UTC))
	lastSuccessfulTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: "1204",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
		},
		Spec: batchv1.CronJobSpec{
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			FailedJobsHistoryLimit:     pointer.Ptr(int32(4)),
			Schedule:                   "*/5 * * * *",
			StartingDeadlineSeconds:    pointer.Ptr(int64(120)),
			SuccessfulJobsHistoryLimit: pointer.Ptr(int32(2)),
			Suspend:                    pointer.Ptr(false),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
							RestartPolicy: corev1.RestartPolicyOnFailure,
						},
					},
				},
			},
		},
		Status: batchv1.CronJobStatus{
			Active: []corev1.ObjectReference{
				{
					APIVersion:      "batch/v1",
					Kind:            "Job",
					Name:            "cronjob-1618585500",
					Namespace:       namespace,
					ResourceVersion: "220593669",
					UID:             "644a62fe-783f-4609-bd2b-a9ec1212c07b",
				},
			},
			LastScheduleTime:   &lastScheduleTime,
			LastSuccessfulTime: &lastSuccessfulTime,
		},
	}
}
