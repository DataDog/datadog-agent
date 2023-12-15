// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains helpers and e2e tests for config subcommand
package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers
func getKeyInMap(config map[interface{}]interface{}, key string) (interface{}, bool) {
	keys := strings.Split(key, ".")
	currentObj := config

	for idx, k := range keys {
		value, ok := currentObj[k]
		if !ok {
			return nil, false
		}

		if nestedMap, isMap := value.(map[interface{}]interface{}); isMap {
			currentObj = nestedMap
		} else if idx == len(keys)-1 {
			return value, true
		} else {
			return nil, false
		}
	}

	return currentObj, true
}

func getConfigValue(t *testing.T, config map[interface{}]interface{}, key string) interface{} {
	value, found := getKeyInMap(config, key)
	require.True(t, found)

	return value
}

func assertConfigValueEqual(t *testing.T, config map[interface{}]interface{}, key string, expectedValue interface{}) {
	value := getConfigValue(t, config, key)
	assert.Equal(t, expectedValue, value)
}

func assertConfigValueContains(t *testing.T, config map[interface{}]interface{}, key string, expectedValue interface{}) {
	value := getConfigValue(t, config, key)
	assert.Contains(t, value, expectedValue)
}
