// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package jsonquery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestYAMLExistQuery(t *testing.T) {
	exist, err := YAMLCheckExist(integration.Data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.50\"")
	assert.NoError(t, err)
	assert.True(t, exist)

	exist, err = YAMLCheckExist(integration.Data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)

	exist, err = YAMLCheckExist(integration.Data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address")
	assert.EqualError(t, err, "filter query must return a boolean: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `127.0.0.50` into bool")
	assert.False(t, exist)

	exist, err = YAMLCheckExist(integration.Data("{}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)
}
