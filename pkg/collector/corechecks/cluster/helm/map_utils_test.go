// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetValue(t *testing.T) {
	tests := []struct {
		name            string
		inputMap        map[string]interface{}
		dotSeparatedKey string
		expectedValue   string
		expectsError    bool
	}{
		{
			name: "standard case",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]string{
						"tag": "7.39.0",
					},
				},
			},
			dotSeparatedKey: "agents.image.tag",
			expectedValue:   "7.39.0",
		},
		{
			name: "non-string value",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]int{
						"tag": 1,
					},
				},
			},
			dotSeparatedKey: "agents.image.tag",
			expectedValue:   "1",
		},
		{
			name: "result is a map",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]string{
						"tag": "7.39.0",
					},
				},
			},
			dotSeparatedKey: "agents.image",
			expectsError:    true,
		},
		{
			name: "input map has less levels than key",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": "datadog/agent:7.39.0",
				},
			},
			dotSeparatedKey: "agents.image.tag",
			expectsError:    true,
		},
		{
			name:            "input map is empty",
			inputMap:        map[string]interface{}{},
			dotSeparatedKey: "agents.image.tag",
			expectsError:    true,
		},
		{
			name: "first element of the key not found",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]string{
						"tag": "7.39.0",
					},
				},
			},
			dotSeparatedKey: "abc.image.tag",
			expectsError:    true,
		},
		{
			name: "some element of the key other than the first not found",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]string{
						"tag": "7.39.0",
					},
				},
			},
			dotSeparatedKey: "agents.image.abc",
			expectsError:    true,
		},
		{
			name: "nil value for final part of the key",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]interface{}{
						"tag": nil,
					},
				},
			},
			dotSeparatedKey: "agents.image.tag",
			expectsError:    true,
		},
		{
			name: "nil value for middle part of the key",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": nil,
				},
			},
			dotSeparatedKey: "agents.image.tag",
			expectsError:    true,
		},
		{
			name: "empty key",
			inputMap: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]string{
						"tag": "7.39.0",
					},
				},
			},
			dotSeparatedKey: "",
			expectsError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := getValue(test.inputMap, test.dotSeparatedKey)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedValue, result)
			}
		})
	}
}
