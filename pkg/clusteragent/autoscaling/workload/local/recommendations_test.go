// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/google/go-cmp/cmp"

	// logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	// workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	// workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	// "github.com/DataDog/datadog-agent/pkg/util/fxutil"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestNewResourceRecommenderSettings(t *testing.T) {
	tests := []struct {
		name   string
		target datadoghq.DatadogPodAutoscalerTarget
		want   *resourceRecommenderSettings
		err    error
	}{
		{
			name: "Pod resource - CPU target utilization",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: &resourceRecommenderSettings{
				MetricName:    "container.cpu.usage",
				LowWatermark:  0.75,
				HighWatermark: 0.85,
			},
			err: nil,
		},
		{
			name: "Pod resource - memory utilization",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "memory",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: &resourceRecommenderSettings{
				MetricName:    "container.memory.usage",
				LowWatermark:  0.75,
				HighWatermark: 0.85,
			},
			err: nil,
		},
		{
			name: "Pod resource - invalid value type",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type: datadoghq.DatadogPodAutoscalerAbsoluteTargetValueType,
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid value type: Absolute"),
		},
		{
			name: "Pod resource - invalid name",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "some-resource",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid resource name: some-resource"),
		},
		{
			name: "Container resource - CPU target utilization",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerContainerResourceTargetType,
				ContainerResource: &datadoghq.DatadogPodAutoscalerContainerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: &resourceRecommenderSettings{
				MetricName:    "container.cpu.usage",
				LowWatermark:  0.75,
				HighWatermark: 0.85,
			},
			err: nil,
		},
		{
			name: "Container resource - memory utilization",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerContainerResourceTargetType,
				ContainerResource: &datadoghq.DatadogPodAutoscalerContainerResourceTarget{
					Name: "memory",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: &resourceRecommenderSettings{
				MetricName:    "container.memory.usage",
				LowWatermark:  0.75,
				HighWatermark: 0.85,
			},
			err: nil,
		},
		{
			name: "Container resource - invalid value type",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerContainerResourceTargetType,
				ContainerResource: &datadoghq.DatadogPodAutoscalerContainerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type: datadoghq.DatadogPodAutoscalerAbsoluteTargetValueType,
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid value type: Absolute"),
		},
		{
			name: "Container resource - invalid name",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerContainerResourceTargetType,
				ContainerResource: &datadoghq.DatadogPodAutoscalerContainerResourceTarget{
					Name: "some-resource",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid resource name: some-resource"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommenderSettings, err := newResourceRecommenderSettings(tt.target)
			if tt.err != nil {
				assert.Error(t, err, tt.err.Error())
			} else {
				assert.NoError(t, err)
				if diff := cmp.Diff(recommenderSettings, tt.want); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestProcessAverageContainerMetricValue(t *testing.T) {
	testTime := time.Now()

	tests := []struct {
		name          string
		series        []EntityValue
		currentTime   time.Time
		averageMetric float64
		lastTimestamp time.Time
		err           error
	}{
		{
			name:          "Empty series",
			series:        []EntityValue{},
			averageMetric: 0.0,
			lastTimestamp: time.Time{},
			err:           fmt.Errorf("Missing usage metrics"),
		},
		{
			name: "Series with valid values (non-stale)",
			series: []EntityValue{
				{
					timestamp: Timestamp(testTime.Unix() - 15),
					value:     ValueType(2),
				},
				{
					timestamp: Timestamp(testTime.Unix() - 30),
					value:     ValueType(3),
				},
				{
					timestamp: Timestamp(testTime.Unix() - 45),
					value:     ValueType(4),
				},
			},
			currentTime:   testTime,
			averageMetric: 3.0,
			lastTimestamp: time.Unix(testTime.Unix()-15, 0),
			err:           nil,
		},
		{
			name: "Series with some stale values",
			series: []EntityValue{
				{
					timestamp: Timestamp(testTime.Unix() - 15),
					value:     ValueType(2),
				},
				{
					timestamp: Timestamp(testTime.Unix() - 30),
					value:     ValueType(4),
				},
				{
					timestamp: Timestamp(testTime.Unix() - 270),
					value:     ValueType(4),
				},
			},
			currentTime:   testTime,
			averageMetric: 3.0,
			lastTimestamp: time.Unix(testTime.Unix()-15, 0),
			err:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			averageMetric, lastTimestamp, err := processAverageContainerMetricValue(tt.series, tt.currentTime)
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

func TestCalculateUtilization(t *testing.T) {
	testTime := time.Now()
	tests := []struct {
		name        string
		pods        []*workloadmeta.KubernetesPod
		queryResult QueryResult
		currentTime time.Time
		want        UtilizationResult
		err         error
	}{
		{
			name:        "Empty pods",
			pods:        []*workloadmeta.KubernetesPod{},
			queryResult: QueryResult{},
			currentTime: time.Time{},
			want:        UtilizationResult{},
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{},
			currentTime: testTime,
			want:        UtilizationResult{},
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]EntityValue{
							"container-1": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(time.Now().Unix()),
								},
								{
									value:     ValueType(3),
									timestamp: Timestamp(time.Now().Unix() - 15),
								},
							},
						},
					},
				},
			},
			currentTime: testTime,
			want:        UtilizationResult{},
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(3),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: UtilizationResult{
				AverageUtilization: 2.5,
				MissingPods:        []string{},
				PodToUtilization: map[string]float64{
					"pod-name1": 2.5,
				},
				RecommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
						{
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(3),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
							"container-name2": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(4),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: UtilizationResult{
				AverageUtilization: 2.75,
				MissingPods:        []string{},
				PodToUtilization: map[string]float64{
					"pod-name1": 2.75,
				},
				RecommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(3),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]EntityValue{
							"container-name2": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(4),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: UtilizationResult{
				AverageUtilization: 2.75,
				MissingPods:        []string{},
				PodToUtilization: map[string]float64{
					"pod-name1": 2.5,
					"pod-name2": 3.0,
				},
				RecommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
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
								CPURequest:    func(f float64) *float64 { return &f }(0.1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(3),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime: testTime,
			want: UtilizationResult{
				AverageUtilization: 2.5,
				MissingPods:        []string{"pod-name2"},
				PodToUtilization: map[string]float64{
					"pod-name1": 2.5,
				},
				RecommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := newResourceRecommenderSettings(datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			})
			assert.NoError(t, err)
			utilization, err := r.calculateUtilization(tt.pods, tt.queryResult, tt.currentTime)
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
			r, err := newResourceRecommenderSettings(datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(tt.targetUtilization),
					},
				},
			})
			assert.NoError(t, err)

			replicas := r.calculateReplicas(tt.currentReplicas, tt.averageUtilization)
			assert.Equal(t, tt.want, replicas)
		})
	}
}

func TestRecommend(t *testing.T) {
	testTime := time.Now()
	tests := []struct {
		name                    string
		pods                    []*workloadmeta.KubernetesPod
		queryResult             QueryResult
		currentTime             time.Time
		currentReplicas         float64
		recommendedReplicas     int32
		recommendationTimestamp time.Time
		err                     error
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
			queryResult:             QueryResult{},
			currentTime:             testTime,
			currentReplicas:         4,
			recommendedReplicas:     0,
			recommendationTimestamp: time.Time{},
			err:                     fmt.Errorf("Issue fetching metrics data"),
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
								CPURequest:    func(f float64) *float64 { return &f }(1),
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name2",
						ContainerValues: map[string][]EntityValue{
							"container-1": []EntityValue{
								{
									value:     ValueType(2),
									timestamp: Timestamp(time.Now().Unix()),
								},
								{
									value:     ValueType(3),
									timestamp: Timestamp(time.Now().Unix() - 15),
								},
							},
						},
					},
				},
			},
			currentTime:             testTime,
			recommendedReplicas:     0,
			recommendationTimestamp: time.Time{},
			currentReplicas:         4,
			err:                     fmt.Errorf("Issue calculating pod utilization"),
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
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(100),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(123),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
							"container-name2": []EntityValue{
								{
									value:     ValueType(140),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(154),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime:             testTime,
			currentReplicas:         4,
			recommendedReplicas:     3,
			recommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
			err:                     nil,
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
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(240),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(230),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
							"container-name2": []EntityValue{
								{
									value:     ValueType(244),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(230),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime:             testTime,
			currentReplicas:         4,
			recommendedReplicas:     5,
			recommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
			err:                     nil,
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
							ID:   "container-id2",
							Name: "container-name2",
							Resources: workloadmeta.ContainerResources{
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(240),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(230),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime:             testTime,
			currentReplicas:         4,
			recommendedReplicas:     4,
			recommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
			err:                     nil,
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
								CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
								MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
							},
						},
					},
				},
			},
			queryResult: QueryResult{
				results: []PodResult{
					{
						PodName: "pod-name1",
						ContainerValues: map[string][]EntityValue{
							"container-name1": []EntityValue{
								{
									value:     ValueType(5),
									timestamp: Timestamp(testTime.Unix() - 15),
								},
								{
									value:     ValueType(15),
									timestamp: Timestamp(testTime.Unix() - 30),
								},
							},
						},
					},
				},
			},
			currentTime:             testTime,
			currentReplicas:         4,
			recommendedReplicas:     3, // original recommendation 1
			recommendationTimestamp: time.Unix(testTime.Unix()-15, 0),
			err:                     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := newResourceRecommenderSettings(datadoghq.DatadogPodAutoscalerTarget{
				Type: datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: &datadoghq.DatadogPodAutoscalerResourceTarget{
					Name: "cpu",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			})
			assert.NoError(t, err)
			recommendedReplicas, recommendationTimestamp, err := r.recommend(tt.currentTime, tt.pods, tt.queryResult, tt.currentReplicas)
			if err != nil {
				assert.Error(t, err, tt.err.Error())
				assert.Equal(t, tt.err, err)
			} else {
				assert.Equal(t, tt.recommendationTimestamp, recommendationTimestamp)
				assert.Equal(t, tt.recommendedReplicas, recommendedReplicas)
			}
		})
	}
}

// func TestCalculateHorizontalRecommendations(t *testing.T) {
// 	test := []struct {
// 		name string
// 		dpai model.PodAutoscalerInternal
// 		want *model.HorizontalScalingValues
// 		err  error
// 	}{}

// 	for _, tt := range test {
// 		t.Run(tt.name, func(t *testing.T) {
// 			wlm := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
// 				fx.Provide(func() log.Component { return logmock.New(t) }),
// 				config.MockModule(),
// 				fx.Supply(context.Background()),
// 				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
// 			))
// 			pw := NewPodWatcher(wlm, nil)
// 			ctx, cancel := context.WithCancel(context.Background())
// 			go pw.Run(ctx)
// 		})
// 	}
// }
