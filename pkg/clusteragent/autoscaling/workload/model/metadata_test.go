// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver && test

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    Annotations
	}{
		{
			name:        "Empty annotations",
			annotations: map[string]string{},
			expected:    Annotations{},
		},
		{
			name: "URL annotation",
			annotations: map[string]string{
				AnnotationsURLKey: "localhost:8080/test",
			},
			expected: Annotations{
				Endpoint: "localhost:8080/test",
			},
		},
		{
			name: "Fallback annotation",
			annotations: map[string]string{
				AnnotationsFallbackURLKey: "localhost:8080/fallback",
			},
			expected: Annotations{
				FallbackEndpoint: "localhost:8080/fallback",
			},
		},
		{
			name: "URL and Fallback annotation",
			annotations: map[string]string{
				AnnotationsURLKey:         "localhost:8080/test",
				AnnotationsFallbackURLKey: "localhost:8080/fallback",
			},
			expected: Annotations{
				Endpoint:         "localhost:8080/test",
				FallbackEndpoint: "localhost:8080/fallback",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedAnnotation := ParseAnnotations(tt.annotations)
			assert.Equal(t, tt.expected, parsedAnnotation)
		})
	}
}
