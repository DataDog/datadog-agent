// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func TestConvertParseDistributionServerless(t *testing.T) {
	defaultHostname, err := hostname.Get(context.Background())

	assert.Equal(t, "", defaultHostname, "In serverless mode, the hostname returned should be an empty string")
	assert.NoError(t, err)
	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:3.5|d"), enrichConfig{})

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, metrics.DistributionType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))

	// this is the important part of the test: hostname.Get() should return
	// an empty string and the parser / enricher should keep the host that way.
	assert.Equal(t, "", parsed.Host, "In serverless mode, the hostname should be an empty string")
}

func TestGetServerlessMetricSource(t *testing.T) {
	tests := []struct {
		name           string
		tags           []string
		expectedSource metrics.MetricSource
	}{
		{
			name:           "AWS Lambda Custom",
			tags:           []string{},
			expectedSource: metrics.MetricSourceAwsLambdaCustom,
		},
		{
			name:           "Azure Container App Custom",
			tags:           []string{},
			expectedSource: metrics.MetricSourceAzureContainerAppCustom,
		},
		{
			name:           "Azure App Service Custom",
			tags:           []string{},
			expectedSource: metrics.MetricSourceAzureAppServiceCustom,
		},
		{
			name:           "Google Cloud Run Custom",
			tags:           []string{},
			expectedSource: metrics.MetricSourceGoogleCloudRunCustom,
		},
		{
			name:           "No change for regular tag",
			tags:           []string{},
			expectedSource: GetDefaultMetricSource(),
		},
		{
			name:           "No change for non-matching prefix",
			tags:           []string{},
			expectedSource: GetDefaultMetricSource(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, result := extractTagsMetadata(tt.tags, "", 0, origindetection.LocalData{}, origindetection.ExternalData{}, "", enrichConfig{})
			assert.Equal(t, tt.expectedSource.String(), result.String())
		})
	}
}

func TestGetServerlessMetricSourceRuntime(t *testing.T) {
	tests := []struct {
		name           string
		tags           []string
		expectedSource metrics.MetricSource
	}{
		{
			name:           "AWS Lambda Runtime",
			tags:           []string{},
			expectedSource: metrics.MetricSourceAwsLambdaRuntime,
		},
		{
			name:           "Azure Container App Runtime",
			tags:           []string{},
			expectedSource: metrics.MetricSourceAzureContainerAppRuntime,
		},
		{
			name:           "Azure App Service Runtime",
			tags:           []string{},
			expectedSource: metrics.MetricSourceAzureAppServiceRuntime,
		},
		{
			name:           "Google Cloud Run Runtime",
			tags:           []string{},
			expectedSource: metrics.MetricSourceGoogleCloudRunRuntime,
		},
		{
			name:           "No change for regular tag",
			tags:           []string{},
			expectedSource: GetDefaultMetricSource(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, result := extractTagsMetadata(tt.tags, "", 0, origindetection.LocalData{}, origindetection.ExternalData{}, "", enrichConfig{})
			assert.Equal(t, tt.expectedSource.String(), result.String())
		})
	}
}
