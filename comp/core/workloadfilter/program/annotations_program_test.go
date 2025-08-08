// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"testing"

	"github.com/stretchr/testify/assert"

	filterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// mockFilterable implements the filterdef.Filterable interface for testing
type mockFilterable struct {
	name        string
	annotations map[string]string
}

func (m *mockFilterable) Serialize() any {
	return nil
}

func (m *mockFilterable) Type() filterdef.ResourceType {
	return filterdef.ContainerType
}

func (m *mockFilterable) GetAnnotations() map[string]string {
	return m.annotations
}

func (m *mockFilterable) GetName() string {
	return m.name
}

func TestAnnotationsProgram_Evaluate(t *testing.T) {
	tests := []struct {
		name           string
		entityName     string
		annotations    map[string]string
		excludePrefix  string
		expectedResult filterdef.Result
		expectedErrors int
	}{
		{
			name:           "No annotations - should return Unknown",
			entityName:     "test-container",
			annotations:    map[string]string{},
			excludePrefix:  "",
			expectedResult: filterdef.Unknown,
			expectedErrors: 0,
		},
		{
			name:       "Global exclude annotation set to true - should be Excluded",
			entityName: "test-container",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "true",
			},
			excludePrefix:  "",
			expectedResult: filterdef.Excluded,
			expectedErrors: 0,
		},
		{
			name:       "Global exclude annotation set to false - should return Unknown",
			entityName: "test-container",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "false",
			},
			excludePrefix:  "",
			expectedResult: filterdef.Unknown,
			expectedErrors: 0,
		},
		{
			name:       "Global exclude annotation set to 1 - should be Excluded",
			entityName: "test-container",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "1",
			},
			excludePrefix:  "",
			expectedResult: filterdef.Excluded,
			expectedErrors: 0,
		},
		{
			name:       "Container-specific exclude annotation set to true - should be Excluded",
			entityName: "web-server",
			annotations: map[string]string{
				"ad.datadoghq.com/web-server.exclude": "true",
			},
			excludePrefix:  "",
			expectedResult: filterdef.Excluded,
			expectedErrors: 0,
		},
		{
			name:       "Container-specific exclude annotation set to false - should return Unknown",
			entityName: "web-server",
			annotations: map[string]string{
				"ad.datadoghq.com/web-server.exclude": "false",
			},
			excludePrefix:  "",
			expectedResult: filterdef.Unknown,
			expectedErrors: 0,
		},
		{
			name:       "Logs exclude prefix - global logs exclude",
			entityName: "nginx-proxy",
			annotations: map[string]string{
				"ad.datadoghq.com/logs_exclude": "true",
			},
			excludePrefix:  "logs_",
			expectedResult: filterdef.Excluded,
			expectedErrors: 0,
		},
		{
			name:       "Wrong prefix should not trigger exclusion",
			entityName: "app-server",
			annotations: map[string]string{
				"ad.datadoghq.com/logs_exclude": "true",
			},
			excludePrefix:  "metrics_", // looking for metrics_ but have logs_
			expectedResult: filterdef.Unknown,
			expectedErrors: 0,
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
			entity := &mockFilterable{
				name:        tt.entityName,
				annotations: tt.annotations,
			}

			// Evaluate the program
			result, errors := program.Evaluate(entity)

			// Assert results
			assert.Equal(t, tt.expectedResult, result, "Expected result does not match")
			assert.Len(t, errors, tt.expectedErrors, "Expected number of errors does not match")
		})
	}
}
