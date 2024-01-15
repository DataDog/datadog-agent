// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLanguageAnnotationKey(t *testing.T) {
	mockContainerName := "some-container-name"
	expectedAnnotationKey := "internal.dd.datadoghq.com/some-container-name.detected_langs"
	actualAnnotationKey := GetLanguageAnnotationKey(mockContainerName)
	assert.Equal(t, expectedAnnotationKey, actualAnnotationKey)
}

func TestExtractContainerFromAnnotationKey(t *testing.T) {
	tests := []struct {
		name            string
		annotationKey   string
		containerName   string
		isInitContainer bool
	}{
		{
			name:            "Non-matching annotation key",
			annotationKey:   "IAmNotALanguageAnnotationKey",
			containerName:   "",
			isInitContainer: false,
		},
		{
			name:            "Standard language annotation",
			annotationKey:   "internal.dd.datadoghq.com/some-container-name.detected_langs",
			containerName:   "some-container-name",
			isInitContainer: false,
		},
		{
			name:            "Language annotation for init container",
			annotationKey:   "internal.dd.datadoghq.com/init.some-container-name.detected_langs",
			containerName:   "some-container-name",
			isInitContainer: true,
		},
		{
			name:            "Language annotation for non-init container whose name starts with init",
			annotationKey:   "internal.dd.datadoghq.com/initializer.detected_langs",
			containerName:   "initializer",
			isInitContainer: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualContainerName, actualIsInit := ExtractContainerFromAnnotationKey(tt.annotationKey)
			assert.Equal(t, tt.containerName, actualContainerName)
			assert.Equal(t, tt.isInitContainer, actualIsInit)
		})
	}
}
