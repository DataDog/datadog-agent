// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestNewResourceRecommenderSettings(t *testing.T) {
	tests := []struct {
		name      string
		objective datadoghqcommon.DatadogPodAutoscalerObjective
		want      *resourceRecommenderSettings
		err       error
	}{
		{
			name: "Invalid resource type",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: "something-invalid",
			},
			want: nil,
			err:  fmt.Errorf("Invalid target type: something-invalid"),
		},
		{
			name: "Pod resource - CPU target utilization",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: &resourceRecommenderSettings{
				metricName:                 "container.cpu.usage",
				lowWatermark:               0.75,
				highWatermark:              0.85,
				fallbackStaleDataThreshold: 60,
			},
			err: nil,
		},
		{
			name: "Pod resource - memory utilization",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "memory",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: &resourceRecommenderSettings{
				metricName:                 "container.memory.usage",
				lowWatermark:               0.75,
				highWatermark:              0.85,
				fallbackStaleDataThreshold: 60,
			},
			err: nil,
		},
		{
			name: "Pod resource - nil target",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type:        datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: nil,
			},
			want: nil,
			err:  fmt.Errorf("nil target"),
		},
		{
			name: "Pod resource - invalid name",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "some-resource",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid resource name: some-resource"),
		},
		{
			name: "Pod resource - nil utilization",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type: datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid utilization value: missing utilization value"),
		},
		{
			name: "Pod resource - out of bounds utilization value",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(0)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid utilization value: utilization value must be between 1 and 100"),
		},
		{
			name: "Container resource - CPU target utilization",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
					Container: "container-foo",
				},
			},
			want: &resourceRecommenderSettings{
				metricName:                 "container.cpu.usage",
				lowWatermark:               0.75,
				highWatermark:              0.85,
				containerName:              "container-foo",
				fallbackStaleDataThreshold: 60,
			},
			err: nil,
		},
		{
			name: "Container resource - memory utilization",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
					Name: "memory",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
					Container: "container-foo",
				},
			},
			want: &resourceRecommenderSettings{
				metricName:                 "container.memory.usage",
				lowWatermark:               0.75,
				highWatermark:              0.85,
				containerName:              "container-foo",
				fallbackStaleDataThreshold: 60,
			},
			err: nil,
		},
		{
			name: "Container resource - nil target",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type:              datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: nil,
			},
			want: nil,
			err:  fmt.Errorf("nil target"),
		},
		{
			name: "Container resource - invalid name",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
					Name: "some-resource",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(80)),
					},
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid resource name: some-resource"),
		},
		{
			name: "Container resource - nil utilization",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type: datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
					},
					Container: "container-foo",
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid utilization value: missing utilization value"),
		},
		{
			name: "Container resource - out of bounds utilization value",
			objective: datadoghqcommon.DatadogPodAutoscalerObjective{
				Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
				ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: pointer.Ptr(int32(0)),
					},
					Container: "container-foo",
				},
			},
			want: nil,
			err:  fmt.Errorf("invalid utilization value: utilization value must be between 1 and 100"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommenderSettings, err := newResourceRecommenderSettings(tt.objective)
			if tt.err != nil {
				assert.Error(t, err, tt.err.Error())
			} else {
				assert.NoError(t, err)
				assert.Empty(t, cmp.Diff(recommenderSettings, tt.want, cmp.AllowUnexported(resourceRecommenderSettings{})))
			}
		})
	}
}
