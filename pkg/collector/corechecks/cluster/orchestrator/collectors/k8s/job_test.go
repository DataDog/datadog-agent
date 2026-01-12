// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestJobCollector(t *testing.T) {
	creationTime := CreateTestTime()
	startTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:      "job",
			Namespace: "project",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "batch/v1beta1",
					Controller: pointer.Ptr(true),
					Kind:       "CronJob",
					Name:       "test-job",
					UID:        "d0326ca4-d405-4fe9-99b5-7bfc4a6722b6",
				},
			},
			ResourceVersion: "1212",
			UID:             types.UID("8893e7a0-fc49-4627-b695-3ed47074ecba"),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: pointer.Ptr(int32(6)),
			Completions:  pointer.Ptr(int32(1)),
			Parallelism:  pointer.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"controller-uid": "43739057-c6d7-4a5e-ab63-d0c8844e5272",
				},
			},
		},
		Status: batchv1.JobStatus{
			Active:    1,
			StartTime: &startTime,
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewJobCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{job},
		ExpectedMetadataType:       &model.CollectorJob{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
