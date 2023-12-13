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
	expectedAnnotationKey := "apm.datadoghq.com/some-container-name.languages"
	actualAnnotationKey := GetLanguageAnnotationKey(mockContainerName)
	assert.Equal(t, expectedAnnotationKey, actualAnnotationKey)
}
