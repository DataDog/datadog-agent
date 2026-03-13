// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
)

func TestAnnotationsProgram_Evaluate(t *testing.T) {
	tests := []struct {
		name           string
		entityName     string
		annotations    map[string]string
		excludePrefix  string
		expectedResult workloadfilter.Result
	}{
		{
			name:           "No annotations - should return Unknown",
			entityName:     "test-container",
			annotations:    map[string]string{},
			excludePrefix:  "",
			expectedResult: workloadfilter.Unknown,
		},
		{
			name:       "Global exclude annotation set to true - should be Excluded",
			entityName: "test-container",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "true",
			},
			excludePrefix:  "",
			expectedResult: workloadfilter.Excluded,
		},
		{
			name:       "Global exclude annotation set to false - should return Unknown",
			entityName: "test-container",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "false",
			},
			excludePrefix:  "",
			expectedResult: workloadfilter.Unknown,
		},
		{
			name:       "Global exclude annotation set to 1 - should be Excluded",
			entityName: "test-container",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "1",
			},
			excludePrefix:  "",
			expectedResult: workloadfilter.Excluded,
		},
		{
			name:       "Container-specific exclude annotation set to true - should be Excluded",
			entityName: "web-server",
			annotations: map[string]string{
				"ad.datadoghq.com/web-server.exclude": "true",
			},
			excludePrefix:  "",
			expectedResult: workloadfilter.Excluded,
		},
		{
			name:       "Container-specific exclude annotation set to false - should return Unknown",
			entityName: "web-server",
			annotations: map[string]string{
				"ad.datadoghq.com/web-server.exclude": "false",
			},
			excludePrefix:  "",
			expectedResult: workloadfilter.Unknown,
		},
		{
			name:       "Logs exclude prefix - global logs exclude",
			entityName: "nginx-proxy",
			annotations: map[string]string{
				"ad.datadoghq.com/logs_exclude": "true",
			},
			excludePrefix:  "logs_",
			expectedResult: workloadfilter.Excluded,
		},
		{
			name:       "Wrong prefix should not trigger exclusion",
			entityName: "app-server",
			annotations: map[string]string{
				"ad.datadoghq.com/logs_exclude": "true",
			},
			excludePrefix:  "metrics_", // looking for metrics_ but have logs_
			expectedResult: workloadfilter.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the AnnotationsProgram
			program := AnnotationsProgram{
				Name:          "TestAnnotationsProgram",
				ExcludePrefix: tt.excludePrefix,
			}

			// Create mock entity
			entity := &mock.Filterable{
				EntityName:        tt.entityName,
				EntityAnnotations: tt.annotations,
				EntityType:        workloadfilter.ContainerType,
			}

			// Evaluate the program
			result := program.Evaluate(entity)

			// Assert results
			assert.Equal(t, tt.expectedResult, result, "Expected result does not match")
		})
	}
}
