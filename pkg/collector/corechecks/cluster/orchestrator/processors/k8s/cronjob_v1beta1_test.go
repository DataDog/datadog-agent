// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestCronJobV1Beta1Handlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.CronJob{}
	resource := &batchv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cronjob",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("batch", "cronjobs")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewCronJobV1Beta1Handlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
}

func TestCronJobV1Beta1Processor_Process(t *testing.T) {
	// Create test cron jobs with unique UIDs
	cronJob1 := createTestCronJobV1Beta1("cronjob-1", "namespace-1")
	cronJob1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	cronJob1.ResourceVersion = "1205"

	cronJob2 := createTestCronJobV1Beta1("cronjob-2", "namespace-2")
	cronJob2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	cronJob2.ResourceVersion = "1305"

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
			APIVersion:       "batch/v1beta1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process cron jobs
	processor := processors.NewProcessor(NewCronJobV1Beta1Handlers(processorstest.NewEmptyFakeTagger()))
	result, listed, processed := processor.Process(ctx, []*batchv1beta1.CronJob{cronJob1, cronJob2})

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

	expectedCronJob1 := k8sTransformers.ExtractCronJobV1Beta1(ctx, cronJob1)

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
	var actualManifestCronJob batchv1beta1.CronJob
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
	assert.Equal(t, cronJob1.Status.Active, actualManifestCronJob.Status.Active)
}

func createTestCronJobV1Beta1(name, namespace string) *batchv1beta1.CronJob {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	lastScheduleTime := metav1.NewTime(time.Date(2021, time.April, 17, 14, 30, 0, 0, time.UTC))
	lastSuccessfulTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &batchv1beta1.CronJob{
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
			ResourceVersion: "1205",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
		},
		Spec: batchv1beta1.CronJobSpec{
			ConcurrencyPolicy:          batchv1beta1.ForbidConcurrent,
			FailedJobsHistoryLimit:     pointer.Ptr(int32(4)),
			Schedule:                   "*/5 * * * *",
			StartingDeadlineSeconds:    pointer.Ptr(int64(120)),
			SuccessfulJobsHistoryLimit: pointer.Ptr(int32(2)),
			Suspend:                    pointer.Ptr(false),
			JobTemplate: batchv1beta1.JobTemplateSpec{
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
		Status: batchv1beta1.CronJobStatus{
			Active: []corev1.ObjectReference{
				{
					APIVersion:      "batch/v1beta1",
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
