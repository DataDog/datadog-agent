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

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/google/go-cmp/cmp"
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
				MetricName:    "kubernetes.pod.cpu.usage.req_pct.dist",
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
					Name: "memory",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid resource name: memory"),
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
				MetricName:    "container.cpu.usage.rec_pct.dist",
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
					Name: "memory",
					Value: datadoghq.DatadogPodAutoscalerTargetValue{
						Type:        datadoghq.DatadogPodAutoscalerUtilizationTargetValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid resource name: memory"),
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
