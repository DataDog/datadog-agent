// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package endpoints

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestStatus(t *testing.T) {
	provider := Provider{}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusAPIKeySuffixAfterUpdate(t *testing.T) {
	config := configmock.New(t)
	config.Set("dd_url", "https://app.example.com", model.SourceAgentRuntime)
	config.Set("api_key", "key1old12345", model.SourceAgentRuntime)
	config.Set("additional_endpoints", map[string][]string{
		"https://intake.example.com":               {"key2old12346", "key3old12347"},
		"https://series.example.com/api/v2/series": {"key4old12348"},
	}, model.SourceAgentRuntime)

	provider := Provider{}
	var buf bytes.Buffer
	assert.NoError(t, provider.Text(false, &buf))
	assert.Contains(t, buf.String(), "https://app.example.com - API Key ending with:\n      - 2345")
	assert.Contains(t, buf.String(), "https://intake.example.com - API Keys ending with:\n      - 2346\n      - 2347")
	assert.Contains(t, buf.String(), "https://series.example.com/api/v2/series - API Key ending with:\n      - 2348")

	config.Set("api_key", "key1new54321", model.SourceAgentRuntime)
	config.Set("additional_endpoints", map[string][]string{
		"https://intake.example.com":               {"key2new54322", "key3new54323"},
		"https://series.example.com/api/v2/series": {"key4new54324"},
	}, model.SourceAgentRuntime)

	buf.Reset()
	assert.NoError(t, provider.Text(false, &buf))
	assert.Contains(t, buf.String(), "https://app.example.com - API Key ending with:\n      - 4321")
	assert.Contains(t, buf.String(), "https://intake.example.com - API Keys ending with:\n      - 4322\n      - 4323")
	assert.Contains(t, buf.String(), "https://series.example.com/api/v2/series - API Key ending with:\n      - 4324")
	for _, oldSuffix := range []string{"2345", "2346", "2347", "2348"} {
		assert.NotContains(t, buf.String(), oldSuffix)
	}
}
