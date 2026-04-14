// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"errors"
	"testing"
)

var testPodJSON = []byte(`{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
        "name": "test-pod",
        "namespace": "default"
    },
    "spec": {
        "containers": [
            {
                "name": "test-container",
                "image": "busybox",
                "command": ["sleep", "3600"]
            }
        ]
    }
}`)

func TestOutputFormat(t *testing.T) {
	tests := []struct {
		name         string
		input        []byte
		outputFormat string
		expectErr    bool
	}{
		{
			name:         "from json to json",
			input:        testPodJSON,
			outputFormat: "json",
			expectErr:    false,
		},
		{
			name:         "from json to yaml",
			input:        testPodJSON,
			outputFormat: "yaml",
			expectErr:    false,
		},
		{
			name:         "unsupported format",
			input:        testPodJSON,
			outputFormat: "xml",
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// test the output format conversion
			_, err := formatOutput(tt.input, tt.outputFormat)

			// we don't compare the output because the yaml output can have unordered fields.
			// we only ensure it does not fail
			if err != nil {
				if !tt.expectErr {
					t.Errorf("exccpected a result, unexpected error: %v", err)
				}

				// with unsuported format we expect an error
				if !errors.Is(err, ErrUnsupportedFormat) {
					t.Errorf("expected ErrUnsupportedFormat, got: %v", err)
				}
			}
		})
	}
}
