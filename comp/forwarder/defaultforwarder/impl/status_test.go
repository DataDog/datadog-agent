// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

func TestJSON(t *testing.T) {
	config := config.NewMock(t)

	provider := statusProvider{
		config: config,
	}

	status := make(map[string]interface{})
	provider.JSON(false, status)

	jsonString, err := json.Marshal(status)
	assert.NoError(t, err)

	stats := status["forwarderStats"].(map[string]interface{})

	assert.Nil(t, stats["forwarder_storage_max_size_in_bytes"])

	assert.NotEqual(t, "", string(jsonString))
}

func TestJSONWith_forwarder_storage_max_size_in_bytes(t *testing.T) {

	overrides := map[string]interface{}{
		"forwarder_storage_max_size_in_bytes": 67,
	}

	provider := statusProvider{
		config: config.NewMockWithOverrides(t, overrides),
	}

	status := make(map[string]interface{})
	provider.JSON(false, status)

	stats := status["forwarderStats"].(map[string]interface{})

	assert.Equal(t, "67", stats["forwarder_storage_max_size_in_bytes"])
}

func TestText(t *testing.T) {
	config := config.NewMock(t)

	provider := statusProvider{
		config: config,
	}

	b := new(bytes.Buffer)
	provider.Text(false, b)

	assert.NotEqual(t, "", b.String())
}

func TestHTML(t *testing.T) {
	config := config.NewMock(t)

	provider := statusProvider{
		config: config,
	}

	b := new(bytes.Buffer)
	provider.HTML(false, b)

	assert.NotEqual(t, "", b.String())
}

func TestForwarderStatusKeepsADPSectionSeparate(t *testing.T) {
	statusInfo := map[string]interface{}{
		"forwarderStats": map[string]interface{}{
			"Transactions": map[string]interface{}{
				"Success":           float64(10),
				"SuccessByEndpoint": map[string]interface{}{"series": float64(8)},
				"Errors":            float64(3),
				"ErrorsByType":      map[string]interface{}{"DNSErrors": float64(3)},
			},
		},
	}

	var text bytes.Buffer
	require.NoError(t, status.RenderText(templatesFS, "forwarder.tmpl", &text, statusInfo))
	assert.Contains(t, text.String(), "Total number (Core Agent + ADP): 10")
	assert.Contains(t, text.String(), "Successes By Endpoint (Core Agent):")
	assert.Contains(t, text.String(), "Errors By Type (Core Agent + mapped ADP types):")
	assert.NotContains(t, text.String(), "ADP Forwarder Transactions")

	var html bytes.Buffer
	require.NoError(t, status.RenderHTML(templatesFS, "forwarderHTML.tmpl", &html, statusInfo))
	assert.Contains(t, html.String(), "Total number (Core Agent + ADP): 10")
	assert.Contains(t, html.String(), "Successes By Endpoint (Core Agent):")
	assert.Contains(t, html.String(), "Errors By Type (Core Agent + mapped ADP types):")
	assert.NotContains(t, html.String(), "ADP Forwarder Transactions")
}
