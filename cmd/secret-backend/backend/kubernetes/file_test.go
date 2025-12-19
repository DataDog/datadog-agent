// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewK8sFileBackendRequiresPath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "missing secrets_path fails",
			config:      map[string]interface{}{},
			expectError: true,
			errorMsg:    "secrets_path is required",
		},
		{
			name: "with secrets_path succeeds",
			config: map[string]interface{}{
				"secrets_path": tmpDir,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewK8sFileBackend(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, backend)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, backend)
				assert.Equal(t, tmpDir, backend.Config.SecretsPath)
			}
		})
	}
}
