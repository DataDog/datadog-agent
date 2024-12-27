// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/google/go-cmp/cmp"

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
				ContainerName: nil,
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
				ContainerName: nil,
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
				ContainerName: nil,
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
				ContainerName: nil,
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

// func TestRecommend(t *testing.T) {
// 	test := []struct {
// 		name                    string
// 		currentTime             time.Time
// 		stats                   Timeseries
// 		currentReplicas         float64
// 		recommendedReplicas     int32
// 		recommendationTimestamp time.Time
// 		highWatermark           float64
// 		lowWatermark            float64
// 		err                     error
// 	}{
// 		{
// 			name:                    "Missing stats",
// 			currentTime:             time.Now(),
// 			stats:                   Timeseries{},
// 			currentReplicas:         1,
// 			recommendedReplicas:     0,
// 			recommendationTimestamp: time.Time{},
// 			highWatermark:           0.85,
// 			lowWatermark:            0.75,
// 			err:                     fmt.Errorf("Failed to process metrics values: Missing usage metrics"),
// 		},
// 		{
// 			name:        "Stale metrics",
// 			currentTime: time.Now(),
// 			stats: Timeseries{
// 				Epochs: []uint64{uint64(time.Now().Add(-270 * time.Second).Unix()), uint64(time.Now().Add(-240 * time.Second).Unix()), uint64(time.Now().Add(-210 * time.Second).Unix())},
// 				Values: []float64{89.2, 90.1, 91.4},
// 			},
// 			currentReplicas:         1,
// 			recommendedReplicas:     0,
// 			recommendationTimestamp: time.Time{},
// 			highWatermark:           0.85,
// 			lowWatermark:            0.75,
// 			err:                     fmt.Errorf("Metrics are stale"),
// 		},
// 	}

// 	for _, tt := range test {
// 		t.Run(tt.name, func(t *testing.T) {
// 			recSettings := &resourceRecommenderSettings{
// 				MetricName:    "kubernetes.pod.cpu.usage.req_pct.dist",
// 				LowWatermark:  tt.lowWatermark,
// 				HighWatermark: tt.highWatermark,
// 			}
// 			recommendedReplicas, recommendationTimestamp, err := recSettings.recommend(tt.currentTime, tt.stats, tt.currentReplicas)
// 			assert.Equal(t, tt.recommendedReplicas, recommendedReplicas)
// 			assert.Equal(t, tt.recommendationTimestamp, recommendationTimestamp)
// 			assert.Equal(t, tt.err, err)
// 		})
// 	}
// }

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
