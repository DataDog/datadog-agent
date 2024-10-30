// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func TestGetConfiguredaTags(t *testing.T) {
	mockConfig := configmock.New(t)

	set1 := []string{"1", "2", "3"}

	mockConfig.SetWithoutSource("tags", set1)
	assert.Equal(t, set1, GetConfiguredTags(mockConfig, false))
}

func TestGetConfiguredaTagsExtraTags(t *testing.T) {
	mockConfig := configmock.New(t)

	set1 := []string{"1", "2", "3"}

	mockConfig.SetWithoutSource("extra_tags", set1)
	assert.Equal(t, set1, GetConfiguredTags(mockConfig, false))
}

func TestGetConfiguredaTagsDSD(t *testing.T) {
	mockConfig := configmock.New(t)

	set1 := []string{"1", "2", "3"}

	mockConfig.SetWithoutSource("dogstatsd_tags", set1)
	assert.Equal(t, []string{}, GetConfiguredTags(mockConfig, false))
	assert.Equal(t, set1, GetConfiguredTags(mockConfig, true))
}

func TestGetConfiguredaTagsCombined(t *testing.T) {
	mockConfig := configmock.New(t)

	set1 := []string{"1", "2", "3"}
	set2 := []string{"4", "5", "6"}
	set3 := []string{"7", "8", "9"}

	mockConfig.SetWithoutSource("tags", set1)
	mockConfig.SetWithoutSource("extra_tags", set2)
	mockConfig.SetWithoutSource("dogstatsd_tags", set3)

	expected := append(set1, set2...)
	assert.Equal(t, expected, GetConfiguredTags(mockConfig, false))

	expected = append(expected, set3...)
	assert.Equal(t, expected, GetConfiguredTags(mockConfig, true))
}
