// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTerminatedPodCollector(t *testing.T) {
	creationTime := CreateTestTime()
	deletionTime := metav1.NewTime(time.Date(2021, time.April, 16, 15, 30, 0, 0, time.UTC))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			DeletionTimestamp: &deletionTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            "test-pod",
			Namespace:       "default",
			ResourceVersion: "1234",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
			// Pod is assigned to a node
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))

	// Create dependencies using fxutil.Test with proper modules
	mockCfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockTagger := taggerfxmock.SetupFakeTagger(t)

	collector := NewTerminatedPodCollector(mockCfg, mockStore, mockTagger, metadataAsTags)

	// Test basic collector setup with RunCollectorTest
	config := CollectorTestConfig{
		Resources:                  []runtime.Object{pod},
		ExpectedMetadataType:       &model.CollectorPod{},
		ExpectedResourcesListed:    0, // TerminatedPodCollector doesn't process resources through Run()
		ExpectedResourcesProcessed: 0,
		ExpectedMetadataMessages:   0,
		ExpectedManifestMessages:   0,
		SetupFn: func(runCfg *collectors.CollectorRunConfig) {
			// Set up the terminated pod informer factory
			runCfg.OrchestratorInformerFactory.TerminatedPodInformerFactory = runCfg.OrchestratorInformerFactory.InformerFactory
		},
		AssertionsFn: func(t *testing.T, runCfg *collectors.CollectorRunConfig, _ *collectors.CollectorRunResult) {
			terminatedPods := []*corev1.Pod{pod}
			processResult, err := collector.Process(runCfg, terminatedPods)
			assert.NoError(t, err)
			assert.NotNil(t, processResult)
			assert.Equal(t, 1, processResult.ResourcesListed)
			assert.Equal(t, 1, processResult.ResourcesProcessed)
			assert.Len(t, processResult.Result.MetadataMessages, 1)
			assert.Len(t, processResult.Result.ManifestMessages, 1)
			assert.IsType(t, &model.CollectorPod{}, processResult.Result.MetadataMessages[0])
			assert.IsType(t, &model.CollectorManifest{}, processResult.Result.ManifestMessages[0])
		},
	}

	RunCollectorTest(t, config, collector)
}
