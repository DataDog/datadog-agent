// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func getLogsConfigKeys(t *testing.T) (config.Component, *LogsConfigKeys) {
	configMock := fxutil.Test[config.Component](
		t,
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: map[string]any{
			"api_key": "1234",
		}}),
	)

	return configMock, defaultLogsConfigKeys(configMock)
}

func TestGetAPIKeyGetter(t *testing.T) {
	configMock, l := getLogsConfigKeys(t)

	assert.Equal(t, "1234", l.getAPIKeyGetter()())

	configMock.SetWithoutSource("api_key", "abcd")
	assert.Equal(t, "abcd", l.getAPIKeyGetter()())

	configMock.SetWithoutSource("logs_config.api_key", "5678")
	assert.Equal(t, "5678", l.getAPIKeyGetter()())
}

func TestGetAdditionalEndpoints(t *testing.T) {
	expected := []unmarshalEndpoint{
		{
			APIKey:     "apiKey2",
			IsReliable: pointer.Ptr(true),
			UseSSL:     pointer.Ptr(true),
			Endpoint: Endpoint{
				Host: "http://localhost1",
				Port: 1234,
			},
		},
		{
			APIKey:     "apiKey3",
			IsReliable: pointer.Ptr(false),
			Endpoint: Endpoint{
				Host: "http://localhost2",
				Port: 5678,
			},
		},
	}

	configMock, l := getLogsConfigKeys(t)

	// Test with a JSON directly set
	jsonString := `[{
			"api_key":     "apiKey2",
			"Host":        "http://localhost1",
			"Port":        1234,
			"is_reliable": true,
			"use_ssl":     true
		},
		{
			"api_key":     "apiKey3",
			"Host":        "http://localhost2",
			"Port":        5678,
			"is_reliable": false
		}]`
	configMock.SetWithoutSource("logs_config.additional_endpoints", jsonString)

	endpoints := l.getAdditionalEndpoints()
	assert.Equal(t, expected, endpoints)

	// Test with a regular setup from the configuration file
	configMock.UnsetForSource("logs_config.additional_endpoints", model.SourceUnknown)
	configMock.SetWithoutSource("logs_config.additional_endpoints",
		[]map[string]interface{}{
			{
				"api_key":     "apiKey2",
				"Host":        "http://localhost1",
				"Port":        1234,
				"is_reliable": true,
				"use_ssl":     true,
			},
			{
				"api_key":     "apiKey3",
				"Host":        "http://localhost2",
				"Port":        5678,
				"is_reliable": false,
			},
		})
	endpoints = l.getAdditionalEndpoints()
	assert.Equal(t, expected, endpoints)
}
