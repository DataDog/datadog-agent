// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestJSON(t *testing.T) {
	mockConfig := config.Mock(t)
	provider := statusProvider{
		config: mockConfig,
	}

	status := make(map[string]interface{})
	provider.JSON(status)

	jsonString, err := json.Marshal(status)
	assert.NoError(t, err)

	stats := status["forwarderStats"].(map[string]interface{})

	assert.Nil(t, stats["forwarder_storage_max_size_in_bytes"])

	assert.NotEqual(t, "", string(jsonString))
}

func TestJSONWith_forwarder_storage_max_size_in_bytes(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("forwarder_storage_max_size_in_bytes", 67)

	provider := statusProvider{
		config: mockConfig,
	}

	status := make(map[string]interface{})
	provider.JSON(status)

	stats := status["forwarderStats"].(map[string]interface{})

	assert.Equal(t, "67", stats["forwarder_storage_max_size_in_bytes"])
}

func TestText(t *testing.T) {
	mockConfig := config.Mock(t)

	provider := statusProvider{
		config: mockConfig,
	}

	b := new(bytes.Buffer)
	provider.Text(b)

	assert.NotEqual(t, "", b.String())
}

func TestHTML(t *testing.T) {
	mockConfig := config.Mock(t)

	provider := statusProvider{
		config: mockConfig,
	}

	b := new(bytes.Buffer)
	provider.HTML(b)

	assert.NotEqual(t, "", b.String())
}
