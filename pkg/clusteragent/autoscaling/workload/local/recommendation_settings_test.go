// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

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
			name: "Invalid resource type",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type: "something-invalid",
			},
			want: nil,
			err:  fmt.Errorf("Invalid target type: something-invalid"),
		},
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
				metricName:    "container.cpu.usage",
				lowWatermark:  0.75,
				highWatermark: 0.85,
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
				metricName:    "container.memory.usage",
				lowWatermark:  0.75,
				highWatermark: 0.85,
			},
			err: nil,
		},
		{
			name: "Pod resource - nil target",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type:        datadoghq.DatadogPodAutoscalerResourceTargetType,
				PodResource: nil,
			},
			want: nil,
			err:  fmt.Errorf("nil target"),
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
					Container: "container-foo",
				},
			},
			want: &resourceRecommenderSettings{
				metricName:    "container.cpu.usage",
				lowWatermark:  0.75,
				highWatermark: 0.85,
				containerName: "container-foo",
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
					Container: "container-foo",
				},
			},
			want: &resourceRecommenderSettings{
				metricName:    "container.memory.usage",
				lowWatermark:  0.75,
				highWatermark: 0.85,
				containerName: "container-foo",
			},
			err: nil,
		},
		{
			name: "Container resource - nil target",
			target: datadoghq.DatadogPodAutoscalerTarget{
				Type:              datadoghq.DatadogPodAutoscalerContainerResourceTargetType,
				ContainerResource: nil,
			},
			want: nil,
			err:  fmt.Errorf("nil target"),
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
					Container: "container-foo",
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
				if diff := cmp.Diff(recommenderSettings, tt.want, cmp.AllowUnexported(resourceRecommenderSettings{})); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}
