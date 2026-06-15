// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func cpuPodObjective(utilization int32) datadoghqcommon.DatadogPodAutoscalerObjective {
	return datadoghqcommon.DatadogPodAutoscalerObjective{
		Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
		PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
			Name: "cpu",
			Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
				Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
				Utilization: pointer.Ptr(utilization),
			},
		},
	}
}

func TestProcessAverageContainerMetricValue(t *testing.T) {
	testTime := time.Now()

	tests := []struct {
		name          string
		series        []loadstore.EntityValue
		currentTime   time.Time
		minValidTime  time.Time
		averageMetric float64
		lastTimestamp time.Time
		err           error
	}{
		{
			name:          "Empty series",
			series:        []loadstore.EntityValue{},
			averageMetric: 0.0,
			lastTimestamp: time.Time{},
			err:           errors.New("Missing usage metrics"),
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
		{
			name: "Stale values are dropped, not force-included to reach the minimum",
			series: []loadstore.EntityValue{
				*newEntityValue(testTime.Unix()-270, 100), // stale, must be excluded
				*newEntityValue(testTime.Unix()-15, 2),
			},
			currentTime:   testTime,
			averageMetric: 0.0,
			lastTimestamp: time.Time{},
			// Only one fresh point remains after dropping the stale one -> below the minimum.
			err: errors.New("Missing usage metrics"),
		},
		{
			name: "All samples within the warmup window are excluded",
			series: []loadstore.EntityValue{
				*newEntityValue(testTime.Unix()-30, 4),
				*newEntityValue(testTime.Unix()-15, 2),
			},
			currentTime:   testTime,
			minValidTime:  time.Unix(testTime.Unix()-10, 0), // pod cleared warmup 10s ago
			averageMetric: 0.0,
			lastTimestamp: time.Time{},
			err:           errors.New("Missing usage metrics"),
		},
		{
			name: "Only post-warmup samples are averaged",
			series: []loadstore.EntityValue{
				*newEntityValue(testTime.Unix()-30, 100), // within warmup window, excluded
				*newEntityValue(testTime.Unix()-8, 4),
				*newEntityValue(testTime.Unix()-4, 2),
			},
			currentTime:   testTime,
			minValidTime:  time.Unix(testTime.Unix()-10, 0),
			averageMetric: 3.0,
			lastTimestamp: time.Unix(testTime.Unix()-8, 0),
			err:           nil,
		},
		{
			name: "Sample exactly at the warmup boundary is kept",
			series: []loadstore.EntityValue{
				*newEntityValue(testTime.Unix()-15, 100), // before the boundary, excluded
				*newEntityValue(testTime.Unix()-10, 4),   // exactly at the boundary, kept
				*newEntityValue(testTime.Unix()-4, 2),
			},
			currentTime:   testTime,
			minValidTime:  time.Unix(testTime.Unix()-10, 0),
			averageMetric: 3.0,
			lastTimestamp: time.Unix(testTime.Unix()-10, 0),
			err:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			averageMetric, lastTimestamp, err := processAverageContainerMetricValue(tt.series, tt.currentTime, tt.minValidTime, defaultStaleDataThresholdSeconds)
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
			err:         errors.New("No pods found"),
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
			err:         errors.New("Issue fetching metrics data"),
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
			err:         errors.New("Issue calculating pod utilization"),
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
				averageUtilization:      0.275,
				missingPods:             0,
				measuredPods:            1,
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
				averageUtilization:      0.275,
				missingPods:             0,
				measuredPods:            1,
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
				averageUtilization:      0.275,
				missingPods:             0,
				measuredPods:            2,
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
				averageUtilization:      0.25,
				missingPods:             1,
				measuredPods:            1,
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markPodsRunningReady(tt.pods, tt.currentTime)
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
			recSettings, err := newResourceRecommenderSettings(objective)
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

// markPodsRunningReady marks every pod Running, Ready and past its warmup window so that
// its metrics are eligible for the utilization calculation. Used by tests that pre-date
// readiness gating and only care about the utilization math.
func markPodsRunningReady(pods []*workloadmeta.KubernetesPod, currentTime time.Time) {
	readyAt := currentTime.Add(-time.Hour)
	for _, p := range pods {
		p.Phase = string(corev1.PodRunning)
		p.Ready = true
		p.ReadyTimestamp = &readyAt
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
			err:         errors.New("No pods found"),
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
			err:         errors.New("Issue fetching metrics data"),
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
			err:         errors.New("Issue calculating pod utilization"),
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
				averageUtilization:      0.25,
				missingPods:             0,
				measuredPods:            1,
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
				averageUtilization:      0.25,
				missingPods:             0,
				measuredPods:            1,
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
				averageUtilization:      0.275,
				missingPods:             0,
				measuredPods:            2,
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
				averageUtilization:      0.25,
				missingPods:             1,
				measuredPods:            1,
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
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
			markPodsRunningReady(tt.pods, tt.currentTime)
			recSettings, err := newResourceRecommenderSettings(objective)
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
			recSettings, err := newResourceRecommenderSettings(datadoghqcommon.DatadogPodAutoscalerObjective{
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
			err:                 errors.New("Issue fetching metrics data"),
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
			err:                 errors.New("Issue calculating pod utilization"),
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
				averageUtilization:      0.46425,
				missingPods:             0,
				measuredPods:            4,
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
				averageUtilization:      0.941,
				missingPods:             0,
				measuredPods:            4,
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
			currentTime: testTime,
			// Two measured pods at 0.94 want 3 replicas; each of the two missing-metrics pods
			// reserves one more slot (neutral fill) -> 5.
			recommendedReplicas: 5,
			utilizationRes: utilizationResult{
				// Only the two pods with usable metrics drive the average.
				averageUtilization:      0.94,
				missingPods:             2,
				measuredPods:            2,
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
			currentTime: testTime,
			// Two measured pods at 0.25 want 1 replica; each of the two missing-metrics pods
			// reserves one more slot (neutral fill) -> 3.
			recommendedReplicas: 3,
			utilizationRes: utilizationResult{
				// Average is over the two ready pods with metrics.
				averageUtilization:      0.25,
				missingPods:             2,
				measuredPods:            2,
				recommendationTimestamp: time.Unix(testTime.Unix()-30, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recSettings, err := newResourceRecommenderSettings(datadoghqcommon.DatadogPodAutoscalerObjective{
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
			markPodsRunningReady(tt.pods, tt.currentTime)
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

func TestCalculateHorizontalRecommendations(t *testing.T) {
	testTime := time.Now()
	deploymentName := "deploymentName"
	ns := "default"

	cpuObjective := datadoghqcommon.DatadogPodAutoscalerObjective{
		Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
		PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
			Name: "cpu",
			Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
				Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
				Utilization: pointer.Ptr(int32(80)),
			},
		},
	}
	customQueryObjective := datadoghqcommon.DatadogPodAutoscalerObjective{
		Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType,
		CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{
			Request: datadoghqcommon.DatadogPodAutoscalerTimeseriesFormulaRequest{
				Formula: "query1",
				Queries: []datadoghqcommon.DatadogPodAutoscalerTimeseriesQuery{{
					Source: datadoghqcommon.DatadogPodAutoscalerMetricsDataSourceMetrics,
					Name:   "a",
					Metrics: &datadoghqcommon.DatadogPodAutoscalerMetricsTimeseriesQuery{
						Query: "foo",
					},
				}},
			},
		},
	}
	testCases := map[string]struct {
		dpaSpec          datadoghq.DatadogPodAutoscalerSpec
		expectError      bool
		expectedReplicas int32
	}{
		"Scale up expected": {
			dpaSpec: datadoghq.DatadogPodAutoscalerSpec{
				TargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind:       "Deployment",
					Name:       deploymentName,
					APIVersion: "apps/v1",
				},
				Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					cpuObjective,
				},
			},
			expectError:      false,
			expectedReplicas: 2,
		},
		"custom query objective with no fallback returns error": {
			dpaSpec: datadoghq.DatadogPodAutoscalerSpec{
				TargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind:       "Deployment",
					Name:       deploymentName,
					APIVersion: "apps/v1",
				},
				Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					customQueryObjective,
				},
			},
			expectError: true,
		},
		"custom query objective with fallback": {
			dpaSpec: datadoghq.DatadogPodAutoscalerSpec{
				TargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind:       "Deployment",
					Name:       deploymentName,
					APIVersion: "apps/v1",
				},
				Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					customQueryObjective,
				},
				Fallback: &datadoghq.DatadogFallbackPolicy{
					Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{cpuObjective},
					},
				},
			},
			expectError:      false,
			expectedReplicas: 2,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Setup podwatcher
			pw := workload.NewPodWatcher(nil, nil)
			pw.HandleEvent(workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: newFakePod(fakePodConfig{
					namespace:      ns,
					podName:        "pod1",
					containerNames: []string{"container-name1"},
					deployment:     deploymentName,
					cpuRequest:     25,
					readyTimestamp: time.Now().Add(-time.Hour),
				}),
			})

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
			defer resetWorkloadMetricStore()
			assert.Len(t, queryResult.Results, 1)

			dpaSpec := tc.dpaSpec
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

			r := newReplicaCalculator(clock.RealClock{}, pw)
			res, err := r.calculateHorizontalRecommendations(dpai, lStore)
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, res)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedReplicas, res.Replicas)
			}
		})
	}
}

func TestIsMeasurablePod(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		pod  *workloadmeta.KubernetesPod
		want bool
	}{
		{
			name: "running, ready, past warmup",
			pod:  newFakePod(fakePodConfig{readyTimestamp: now.Add(-time.Hour)}),
			want: true,
		},
		{
			name: "pending pod is excluded",
			pod:  newFakePod(fakePodConfig{phase: string(corev1.PodPending)}),
			want: false,
		},
		{
			name: "running but not ready",
			pod:  newFakePod(fakePodConfig{}),
			want: false,
		},
		{
			name: "running, ready, but still within warmup window",
			pod:  newFakePod(fakePodConfig{readyTimestamp: now.Add(-10 * time.Second)}),
			want: false,
		},
		{
			name: "running, ready, but readiness time unknown",
			pod:  newFakePod(fakePodConfig{ready: true}),
			want: false,
		},
		{
			name: "failed pod is excluded",
			pod:  newFakePod(fakePodConfig{phase: string(corev1.PodFailed)}),
			want: false,
		},
		{
			name: "succeeded pod is excluded",
			pod:  newFakePod(fakePodConfig{phase: string(corev1.PodSucceeded)}),
			want: false,
		},
		{
			name: "terminating pod is excluded",
			pod: func() *workloadmeta.KubernetesPod {
				p := newFakePod(fakePodConfig{readyTimestamp: now.Add(-time.Hour)})
				deletion := now.Add(-time.Second)
				p.DeletionTimestamp = &deletion
				return p
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isMeasurablePod(tt.pod, now))
		})
	}
}

// TestCalculateUtilizationExcludesIneligiblePods verifies that non-running pods, pods
// without data, pods with only stale data, and pods still in their warmup window are all
// excluded from the average, while the single healthy pod drives it.
func TestCalculateUtilizationExcludesIneligiblePods(t *testing.T) {
	currentTime := time.Now()
	recSettings, err := newResourceRecommenderSettings(cpuPodObjective(80))
	require.NoError(t, err)

	pods := []*workloadmeta.KubernetesPod{
		newFakePod(fakePodConfig{podName: "healthy", readyTimestamp: currentTime.Add(-time.Hour)}),
		newFakePod(fakePodConfig{podName: "warming", readyTimestamp: currentTime.Add(-5 * time.Second)}),
		newFakePod(fakePodConfig{podName: "pending", phase: string(corev1.PodPending)}),
		newFakePod(fakePodConfig{podName: "no-data", readyTimestamp: currentTime.Add(-time.Hour)}),
		newFakePod(fakePodConfig{podName: "stale", readyTimestamp: currentTime.Add(-time.Hour)}),
	}

	queryResult := loadstore.QueryResult{
		Results: []loadstore.PodResult{
			// Healthy pod: fresh, post-warmup samples at 50% utilization.
			newCPUUsageResult("healthy", "c", 0.5, currentTime),
			// Warming pod: high startup burst, but entirely within its warmup window.
			newCPUUsageResult("warming", "c", 4.0, currentTime),
			// Stale pod: data points are older than the staleness threshold.
			{
				PodName: "stale",
				ContainerValues: map[string][]loadstore.EntityValue{
					"c": {
						*newEntityValue(currentTime.Unix()-300, 4.0e8),
						*newEntityValue(currentTime.Unix()-280, 4.0e8),
					},
				},
			},
			// "no-data" and "pending" have no entries at all.
		},
	}

	res, err := calculateUtilization(*recSettings, pods, queryResult, currentTime)
	require.NoError(t, err)

	// Only the healthy pod contributes to the average.
	assert.Equal(t, 1, res.measuredPods)
	assert.InDelta(t, 0.5, res.averageUtilization, 1e-9)
	// no-data and stale running pods are Ready-but-unmeasured -> missing (reserved as slots).
	// Pending and still-warming pods are excluded entirely.
	assert.Equal(t, 2, res.missingPods)
}

// TestRecommendDoesNotInfinitelyUpscaleWithWarmupPods reproduces the fallback runaway:
// established pods sit right at target, but every scale-up adds a freshly-created pod whose
// startup burst reads far above target. Without warmup gating, that burst inflates the average
// and triggers another scale-up, looping until max replicas. With gating, the warming pod is
// excluded entirely and the recommendation holds at the established size.
//
// This test fails against the pre-fix recommender (replicas climb every iteration) and
// passes once warming-up pods are excluded.
func TestRecommendDoesNotInfinitelyUpscaleWithWarmupPods(t *testing.T) {
	currentTime := time.Now()
	recSettings, err := newResourceRecommenderSettings(cpuPodObjective(80))
	require.NoError(t, err)

	// targetUtilization sits within the [0.75, 0.85] dead band so a correct
	// recommender holds; warmupBurstUtilization is the startup CPU burst of a
	// freshly-created pod (counted at full weight by the pre-fix recommender).
	const (
		targetUtilization      = 0.80
		warmupBurstUtilization = 3.0
		initialReplicas        = 3
		iterations             = 25
	)

	establishedReplicas := initialReplicas
	maxObserved := initialReplicas

	for i := 0; i < iterations; i++ {
		pods := make([]*workloadmeta.KubernetesPod, 0, establishedReplicas+1)
		results := make([]loadstore.PodResult, 0, establishedReplicas+1)

		// Established pods: long ready, sitting right at the target utilization.
		for j := 0; j < establishedReplicas; j++ {
			name := fmt.Sprintf("ready-%d", j)
			pods = append(pods, newFakePod(fakePodConfig{podName: name, readyTimestamp: currentTime.Add(-time.Hour)}))
			results = append(results, newCPUUsageResult(name, "c", targetUtilization, currentTime))
		}
		// One freshly-created pod still within its warmup window, reporting a startup burst.
		pods = append(pods, newFakePod(fakePodConfig{podName: "warmup", readyTimestamp: currentTime.Add(-10 * time.Second)}))
		results = append(results, newCPUUsageResult("warmup", "c", warmupBurstUtilization, currentTime))

		rec, _, err := recommend(currentTime, *recSettings, pods, loadstore.QueryResult{Results: results})
		require.NoError(t, err)

		// The warming pod is excluded entirely, so the recommendation is sized off the
		// established pods alone (at target -> no change). A recommender that counted the
		// warmup pod's burst would climb by one every cycle, up to max replicas.
		if int(rec) > establishedReplicas {
			establishedReplicas = int(rec)
		}
		if establishedReplicas > maxObserved {
			maxObserved = establishedReplicas
		}
	}

	assert.Equalf(t, initialReplicas, establishedReplicas,
		"recommender kept scaling up off warm-up pods, reaching %d replicas (expected to hold at %d)", maxObserved, initialReplicas)
}

// TestRecommendSizesToMeasuredLoadDuringRollout verifies the recommender ignores warming pods
// and sizes purely to the measured load during an in-flight scale-up. With 3 established pods
// at target and 2 still-warming pods, it recommends 3 (the measured count) rather than holding
// at a larger desired count — smoothing that rollout transient is the controller's
// stabilization window's job, not the recommender's.
func TestRecommendSizesToMeasuredLoadDuringRollout(t *testing.T) {
	currentTime := time.Now()
	recSettings, err := newResourceRecommenderSettings(cpuPodObjective(80))
	require.NoError(t, err)

	const (
		establishedReplicas = 3
		warmingReplicas     = 2
		targetUtilization   = 0.80
		warmupBurst         = 3.0
	)

	pods := []*workloadmeta.KubernetesPod{}
	results := []loadstore.PodResult{}
	for j := 0; j < establishedReplicas; j++ {
		name := fmt.Sprintf("ready-%d", j)
		pods = append(pods, newFakePod(fakePodConfig{podName: name, readyTimestamp: currentTime.Add(-time.Hour)}))
		results = append(results, newCPUUsageResult(name, "c", targetUtilization, currentTime))
	}
	// The pods the in-flight scale-up added are still within their warmup window.
	for j := 0; j < warmingReplicas; j++ {
		name := fmt.Sprintf("warmup-%d", j)
		pods = append(pods, newFakePod(fakePodConfig{podName: name, readyTimestamp: currentTime.Add(-10 * time.Second)}))
		results = append(results, newCPUUsageResult(name, "c", warmupBurst, currentTime))
	}

	rec, _, err := recommend(currentTime, *recSettings, pods, loadstore.QueryResult{Results: results})
	require.NoError(t, err)

	assert.Equalf(t, int32(establishedReplicas), rec,
		"expected the recommender to size to the %d measured pods at target, got %d", establishedReplicas, rec)
}

// TestRecommendScalesUpFromHotMeasuredPods verifies the recommendation is sized off the
// measured pods only, with no clamp to any desired count. Three measured pods hot at 120% of
// request want 5 replicas (ceil(1.2/0.85*3)); the warming pod is excluded and does not dampen
// the result.
func TestRecommendScalesUpFromHotMeasuredPods(t *testing.T) {
	currentTime := time.Now()
	recSettings, err := newResourceRecommenderSettings(cpuPodObjective(80))
	require.NoError(t, err)

	pods := []*workloadmeta.KubernetesPod{
		newFakePod(fakePodConfig{podName: "ready-0", readyTimestamp: currentTime.Add(-time.Hour)}),
		newFakePod(fakePodConfig{podName: "ready-1", readyTimestamp: currentTime.Add(-time.Hour)}),
		newFakePod(fakePodConfig{podName: "ready-2", readyTimestamp: currentTime.Add(-time.Hour)}),
		newFakePod(fakePodConfig{podName: "warmup", readyTimestamp: currentTime.Add(-10 * time.Second)}),
	}
	results := []loadstore.PodResult{
		newCPUUsageResult("ready-0", "c", 1.2, currentTime),
		newCPUUsageResult("ready-1", "c", 1.2, currentTime),
		newCPUUsageResult("ready-2", "c", 1.2, currentTime),
		newCPUUsageResult("warmup", "c", 3.0, currentTime),
	}

	rec, _, err := recommend(currentTime, *recSettings, pods, loadstore.QueryResult{Results: results})
	require.NoError(t, err)

	assert.Equalf(t, int32(5), rec, "expected scale-up to 5 off the hot measured pods, got %d", rec)
}

// TestRecommendReservesSlotForMissingMetricsPod verifies the neutral fill: a Running & Ready
// pod whose metrics are absent is reserved as one capacity slot — it neither drives a scale-up
// (no observed load) nor is scaled away (its slot is kept until metrics return).
func TestRecommendReservesSlotForMissingMetricsPod(t *testing.T) {
	currentTime := time.Now()
	recSettings, err := newResourceRecommenderSettings(cpuPodObjective(80))
	require.NoError(t, err)

	// 2 measured pods at target want 2 replicas; the missing pod reserves one more -> 3.
	t.Run("at target reserves the missing slot", func(t *testing.T) {
		pods := []*workloadmeta.KubernetesPod{
			newFakePod(fakePodConfig{podName: "measured-0", readyTimestamp: currentTime.Add(-time.Hour)}),
			newFakePod(fakePodConfig{podName: "measured-1", readyTimestamp: currentTime.Add(-time.Hour)}),
			newFakePod(fakePodConfig{podName: "missing", readyTimestamp: currentTime.Add(-time.Hour)}),
		}
		results := []loadstore.PodResult{
			newCPUUsageResult("measured-0", "c", 0.80, currentTime),
			newCPUUsageResult("measured-1", "c", 0.80, currentTime),
			// "missing" has no metrics.
		}
		rec, res, err := recommend(currentTime, *recSettings, pods, loadstore.QueryResult{Results: results})
		require.NoError(t, err)
		assert.Equal(t, 1, res.missingPods)
		assert.Equalf(t, int32(3), rec, "expected 2 measured + 1 reserved slot = 3, got %d", rec)
	})

	// 3 idle measured pods collapse to 1; the missing pod keeps its slot -> 2, instead of being
	// scaled away.
	t.Run("on scale-down keeps the missing slot", func(t *testing.T) {
		pods := []*workloadmeta.KubernetesPod{
			newFakePod(fakePodConfig{podName: "measured-0", readyTimestamp: currentTime.Add(-time.Hour)}),
			newFakePod(fakePodConfig{podName: "measured-1", readyTimestamp: currentTime.Add(-time.Hour)}),
			newFakePod(fakePodConfig{podName: "measured-2", readyTimestamp: currentTime.Add(-time.Hour)}),
			newFakePod(fakePodConfig{podName: "missing", readyTimestamp: currentTime.Add(-time.Hour)}),
		}
		results := []loadstore.PodResult{
			newCPUUsageResult("measured-0", "c", 0.2, currentTime),
			newCPUUsageResult("measured-1", "c", 0.2, currentTime),
			newCPUUsageResult("measured-2", "c", 0.2, currentTime),
			// "missing" has no metrics.
		}
		rec, res, err := recommend(currentTime, *recSettings, pods, loadstore.QueryResult{Results: results})
		require.NoError(t, err)
		assert.Equal(t, 1, res.missingPods)
		assert.Equalf(t, int32(2), rec, "expected 1 (collapsed measured) + 1 reserved slot = 2, got %d", rec)
	})
}
