// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestProcessAverageContainerMetricValue(t *testing.T) {
	testTime := time.Now()

	tests := []struct {
		name          string
		series        []loadstore.EntityValue
		currentTime   time.Time
		averageMetric float64
		lastTimestamp time.Time
		err           error
	}{
		{
			name:          "Empty series",
			series:        []loadstore.EntityValue{},
			averageMetric: 0.0,
			lastTimestamp: time.Time{},
			err:           fmt.Errorf("Missing usage metrics"),
		},
		{
			name: "Series with valid values (non-stale)",
			series: []loadstore.EntityValue{
				*newEntityValue(testTime.Unix()-45, 4),
				*newEntityValue(testTime.Unix()-30, 3),
				*newEntityValue(testTime.Unix()-15, 2),
			},
			currentTime:   testTime,
			averageMetric: 3.0,
			lastTimestamp: time.Unix(testTime.Unix()-45, 0),
			err:           nil,
		},
		{
			name: "Series with some stale values",
			series: []loadstore.EntityValue{
				*newEntityValue(testTime.Unix()-270, 4),
				*newEntityValue(testTime.Unix()-30, 4),
				*newEntityValue(testTime.Unix()-15, 2),
			},
			currentTime:   testTime,
			averageMetric: 3.0,
			lastTimestamp: time.Unix(testTime.Unix()-30, 0),
			err:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			averageMetric, lastTimestamp, err := processAverageContainerMetricValue(tt.series, tt.currentTime, defaultStaleDataThresholdSeconds)
			if err != nil {
				assert.Error(t, err, tt.err.Error())
				assert.Equal(t, tt.err, err)
			} else {
				assert.Equal(t, tt.averageMetric, averageMetric)
				assert.Equal(t, tt.lastTimestamp, lastTimestamp)
			}
		})
	}
}

func TestCalculateUtilizationPodResource(t *testing.T) {
	testTime := time.Now()
	tests := []struct {
		name        string
		pods        []*workloadmeta.KubernetesPod
		queryResult loadstore.QueryResult
		currentTime time.Time
		want        utilizationResult
		err         error
	}{
		{
			name:        "Empty pods",
			pods:        []*workloadmeta.KubernetesPod{},
			queryResult: loadstore.QueryResult{},
			currentTime: time.Time{},
			want:        utilizationResult{},
			err:         fmt.Errorf("No pods found"),
		},
		{
			name: "Pods with empty query results",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID: "container-id1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{},
			currentTime: testTime,
			want:        utilizationResult{},
			err:         fmt.Errorf("Issue fetching metrics data"),
		},
		{
			name: "Pods with no corresponding metrics data",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID: "container-id1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-1": {
								*newEntityValue(testTime.Unix(), 2e8),
								*newEntityValue(testTime.Unix()-15, 3e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want:        utilizationResult{},
			err:         fmt.Errorf("Issue calculating pod utilization"),
		},
		{
			name: "Single pod and container",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.5e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.275,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.275,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Single pod, multiple containers",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
							"container-name2": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 4e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.275,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": .275,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Multiple single-container pods",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
						},
					},
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name2": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 4e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.275,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.25,
					"pod-name2": 0.30,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Query results missing pod",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.25,
				missingPods:        []string{"pod-name2"},
				podToUtilization: map[string]float64{
					"pod-name1": 0.25,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallbackPolicy := &datadoghq.DatadogFallbackPolicy{
				Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
					Enabled: true,
				},
			}
			objective := datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			}
			recSettings, err := newResourceRecommenderSettings(fallbackPolicy, objective)
			assert.NoError(t, err)
			utilization, err := calculateUtilization(*recSettings, tt.pods, tt.queryResult, tt.currentTime)
			if err != nil {
				assert.Error(t, err, tt.err.Error())
				assert.Equal(t, tt.err, err)
			} else {
				assert.Equal(t, tt.want, utilization)
			}
		})
	}
}

func TestCalculateUtilizationContainerResource(t *testing.T) {
	testTime := time.Now()
	tests := []struct {
		name        string
		pods        []*workloadmeta.KubernetesPod
		queryResult loadstore.QueryResult
		currentTime time.Time
		want        utilizationResult
		err         error
	}{
		{
			name:        "Empty pods",
			pods:        []*workloadmeta.KubernetesPod{},
			queryResult: loadstore.QueryResult{},
			currentTime: time.Time{},
			want:        utilizationResult{},
			err:         fmt.Errorf("No pods found"),
		},
		{
			name: "Pods with empty query results",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID: "container-id1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{},
			currentTime: testTime,
			want:        utilizationResult{},
			err:         fmt.Errorf("Issue fetching metrics data"),
		},
		{
			name: "Pods with no corresponding metrics data",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID: "container-id1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-1": {
								*newEntityValue(testTime.Unix(), 2e8),
								*newEntityValue(testTime.Unix()-15, 3e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want:        utilizationResult{},
			err:         fmt.Errorf("Issue calculating pod utilization"),
		},
		{
			name: "Single pod and container",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.25,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.25,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Single pod, multiple containers",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
							"container-name2": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 4e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.25,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.25,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Multiple single-container pods",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1-1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1-2",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
						},
					},
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 4e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.275,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.25,
					"pod-name2": 0.30,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Query results missing pod",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1-2",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2e8),
								*newEntityValue(testTime.Unix()-30, 3e8),
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: utilizationResult{
				averageUtilization: 0.25,
				missingPods:        []string{"pod-name2"},
				podToUtilization: map[string]float64{
					"pod-name1": 0.25,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		fallbackPolicy := &datadoghq.DatadogFallbackPolicy{
			Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
				Enabled: true,
			},
		}
		objective := datadoghqcommon.DatadogPodAutoscalerObjective{
			Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
			ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
				Name: "cpu",
				Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
					Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					Utilization: pointer.Ptr(int32(80)),
				},
				Container: "container-name1",
			},
		}
		t.Run(tt.name, func(t *testing.T) {
			recSettings, err := newResourceRecommenderSettings(fallbackPolicy, objective)
			assert.NoError(t, err)
			utilization, err := calculateUtilization(*recSettings, tt.pods, tt.queryResult, tt.currentTime)
			if err != nil {
				assert.Error(t, err, tt.err.Error())
				assert.Equal(t, tt.err, err)
			} else {
				assert.Equal(t, tt.want, utilization)
			}
		})
	}
}

func TestCalculateReplicas(t *testing.T) {
	test := []struct {
		name               string
		currentReplicas    float64
		averageUtilization float64
		targetUtilization  int32
		want               int32
	}{
		{
			name:               "Utilization within target range",
			currentReplicas:    4.0,
			averageUtilization: 0.80,
			targetUtilization:  80, // watermark 0.75-0.85
			want:               4,
		},
		{
			name:               "Utilization greater than high watermark",
			currentReplicas:    4.0,
			averageUtilization: 0.90,
			targetUtilization:  80, // watermark 0.75-0.85
			want:               5,
		},
		{
			name:               "Utilization slightly than low watermark, no change",
			currentReplicas:    4.0,
			averageUtilization: 0.70,
			targetUtilization:  80, // watermark 0.75-0.85
			want:               4,
		},
		{
			name:               "Utilization much less than low watermark, decrease replica count",
			currentReplicas:    4.0,
			averageUtilization: 0.20,
			targetUtilization:  80, // watermark 0.75-0.85
			want:               1,
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			recSettings, err := newResourceRecommenderSettings(nil, datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(tt.targetUtilization),
					},
				},
			})
			assert.NoError(t, err)

			replicas := calculateReplicas(*recSettings, tt.currentReplicas, tt.averageUtilization)
			assert.Equal(t, tt.want, replicas)
		})
	}
}

func TestRecommend(t *testing.T) {
	testTime := time.Now()
	tests := []struct {
		name                string
		pods                []*workloadmeta.KubernetesPod
		queryResult         loadstore.QueryResult
		currentTime         time.Time
		recommendedReplicas int32
		utilizationRes      utilizationResult
		err                 error
	}{
		{
			name: "Pods with empty query results",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID: "container-id1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult:         loadstore.QueryResult{},
			currentTime:         testTime,
			recommendedReplicas: 0,
			utilizationRes:      utilizationResult{},
			err:                 fmt.Errorf("Issue fetching metrics data"),
		},
		{
			name: "Pods with no corresponding metrics data",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID: "container-id1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(100), // 1
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-1": {
								*newEntityValue(testTime.Unix(), 2e8),
								*newEntityValue(testTime.Unix()-15, 3e8),
							},
						},
					},
				},
			},
			currentTime:         testTime,
			recommendedReplicas: 0,
			utilizationRes:      utilizationResult{},
			err:                 fmt.Errorf("Issue calculating pod utilization"),
		},
		{
			name: "Scale down expected",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-two",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-two",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name3",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-two",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name4",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 1e8),
								*newEntityValue(testTime.Unix()-30, 1.23e8),
							},
							"container-name2": {
								*newEntityValue(testTime.Unix()-15, 1.4e8),
								*newEntityValue(testTime.Unix()-30, 1.54e8),
							},
						},
					},
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 1e8),
								*newEntityValue(testTime.Unix()-30, 1.1e8),
							},
						},
					},
					{
						PodName: "pod-name3",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 1.1e8),
								*newEntityValue(testTime.Unix()-30, 1.1e8),
							},
						},
					},
					{
						PodName: "pod-name4",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 1.2e8),
								*newEntityValue(testTime.Unix()-30, 1.2e8),
							},
						},
					},
				},
			},
			currentTime:         testTime,
			recommendedReplicas: 3,
			utilizationRes: utilizationResult{
				averageUtilization: 0.46425,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.517,
					"pod-name2": 0.420,
					"pod-name3": 0.44,
					"pod-name4": 0.48,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Scale up expected",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-two",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-three",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name3",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-four",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name4",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.4e8),
								*newEntityValue(testTime.Unix()-30, 2.3e8),
							},
							"container-name2": {
								*newEntityValue(testTime.Unix()-15, 2.44e8),
								*newEntityValue(testTime.Unix()-30, 2.3e8),
							},
						},
					},
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.4e8),
								*newEntityValue(testTime.Unix()-30, 2.2e8),
							},
						},
					},
					{
						PodName: "pod-name3",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.3e8),
								*newEntityValue(testTime.Unix()-30, 2.4e8),
							},
						},
					},
					{
						PodName: "pod-name4",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.4e8),
								*newEntityValue(testTime.Unix()-30, 2.4e8),
							},
						},
					},
				},
			},
			currentTime:         testTime,
			recommendedReplicas: 5,
			utilizationRes: utilizationResult{
				averageUtilization: 0.941,
				missingPods:        []string{},
				podToUtilization: map[string]float64{
					"pod-name1": 0.944,
					"pod-name2": 0.92,
					"pod-name3": 0.94,
					"pod-name4": 0.96,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Missing pod data reverses scaleDirection",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name3",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name4",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.4e8),
								*newEntityValue(testTime.Unix()-30, 2.3e8),
							},
						},
					},
					{
						PodName: "pod-name3",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 2.4e8),
								*newEntityValue(testTime.Unix()-30, 2.3e8),
							},
						},
					},
				},
			},
			currentTime:         testTime,
			recommendedReplicas: 4,
			utilizationRes: utilizationResult{
				averageUtilization: 0.94,
				missingPods:        []string{"pod-name2", "pod-name4"},
				podToUtilization: map[string]float64{
					"pod-name1": 0.94,
					"pod-name2": 0.0,
					"pod-name3": 0.94,
					"pod-name4": 0.0,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
		{
			name: "Missing pod data changes recommendation",
			pods: []*workloadmeta.KubernetesPod{
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name1",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-two",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name2",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-three",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name3",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
				{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "foo-bar-four",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod-name4",
						Namespace: "default",
					},
					Containers: []workloadmeta.OrchestratorContainer{
						{
							ID:   "container-id1",
							Name: "container-name1",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: loadstore.QueryResult{
				Results: []loadstore.PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 0.7e8),
								*newEntityValue(testTime.Unix()-30, 0.5e8),
							},
						},
					},
					{
						PodName: "pod-name4",
						ContainerValues: map[string][]loadstore.EntityValue{
							"container-name1": {
								*newEntityValue(testTime.Unix()-15, 0.6e8),
								*newEntityValue(testTime.Unix()-30, 0.7e8),
							},
						},
					},
				},
			},
			currentTime:         testTime,
			recommendedReplicas: 3, // original recommendation 1
			utilizationRes: utilizationResult{
				averageUtilization: 0.625,
				missingPods:        []string{"pod-name2", "pod-name3"},
				podToUtilization: map[string]float64{
					"pod-name1": 0.24,
					"pod-name2": 1.0,
					"pod-name3": 1.0,
					"pod-name4": 0.26,
				},
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recSettings, err := newResourceRecommenderSettings(nil, datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			})
			assert.NoError(t, err)
			recommendedReplicas, utilizationRes, err := recommend(tt.currentTime, *recSettings, tt.pods, tt.queryResult)
			if err != nil {
				assert.Error(t, err, tt.err.Error())
				assert.Equal(t, tt.err, err)
			} else {
				assert.Equal(t, tt.utilizationRes, utilizationRes)
				assert.Equal(t, tt.recommendedReplicas, recommendedReplicas)
			}
		})
	}
}

func TestCalculateHorizontalRecommendationsScaleUp(t *testing.T) {
	testTime := time.Now()
	deploymentName := "deploymentName"
	ns := "default"

	// Setup podwatcher
	pw := workload.NewPodWatcher(nil, nil)
	pw.HandleEvent(newFakeWLMPodEvent(ns, deploymentName, "pod1", []string{"container-name1"}))

	expectedOwner := workload.NamespacedPodOwner{
		Namespace: ns,
		Kind:      kubernetes.DeploymentKind,
		Name:      deploymentName,
	}
	pods := pw.GetPodsForOwner(expectedOwner)
	assert.Len(t, pods, 1)

	// Setup loadstore
	lStore := loadstore.GetWorkloadMetricStore(context.TODO())
	entities := make(map[*loadstore.Entity]*loadstore.EntityValue)
	entity := newEntity("container.cpu.usage", ns, deploymentName, "pod1", "container-name1")
	entities[entity] = newEntityValue(testTime.Unix()-30, 2.4e8)
	lStore.SetEntitiesValues(entities)
	entities[entity] = newEntityValue(testTime.Unix()-15, 2.45e8)
	lStore.SetEntitiesValues(entities)
	queryResult := lStore.GetMetricsRaw("container.cpu.usage", ns, deploymentName, "")
	assert.Len(t, queryResult.Results, 1)

	dpaSpec := datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: autoscalingv2.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       deploymentName,
			APIVersion: "apps/v1",
		},
		Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
		Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
			{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
		},
	}
	dpa := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: ns,
		},
		Spec: dpaSpec,
		Status: datadoghqcommon.DatadogPodAutoscalerStatus{
			Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{},
		},
	}
	dpai := model.NewPodAutoscalerInternal(dpa)

	r := newReplicaCalculator(pw)
	res, err := r.calculateHorizontalRecommendations(dpai, lStore)
	assert.NoError(t, err)
	assert.Equal(t, int32(2), res.Replicas)

	// cleanup
	resetWorkloadMetricStore()
}
