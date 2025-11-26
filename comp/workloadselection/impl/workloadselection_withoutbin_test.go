// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux && !windows

package workloadselectionimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// TestNewComponent_WithoutBin tests that the component is created but RCListener is not enabled on platforms without the compile policy binary
func TestNewComponent_WithoutBin(t *testing.T) {
	tests := []struct {
		name                     string
		workloadSelectionEnabled bool
		expectRCListener         bool
	}{
		{
			name:                     "workload selection enabled but not on Linux",
			workloadSelectionEnabled: true,
			expectRCListener:         false,
		},
		{
			name:                     "workload selection disabled",
			workloadSelectionEnabled: false,
			expectRCListener:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup temp directory for test
			tempDir := t.TempDir()

			// Override getInstallPath for testing
			originalGetInstallPath := getInstallPath
			getInstallPath = func() string { return tempDir }
			t.Cleanup(func() {
				getInstallPath = originalGetInstallPath
			})

			// Create mock components
			mockConfig := config.NewMock(t)
			mockConfig.SetWithoutSource("apm_config.workload_selection", tt.workloadSelectionEnabled)
			mockLog := logmock.New(t)

			reqs := Requires{
				Log:    mockLog,
				Config: mockConfig,
			}

			// Create component
			provides, err := NewComponent(reqs)
			require.NoError(t, err)
			assert.NotNil(t, provides.Comp)

			// On non-Linux platforms, RCListener should never be created
			// because dd-compile-policy is only available on Linux
			hasListener := len(provides.RCListener.ListenerProvider) > 0
			assert.Equal(t, tt.expectRCListener, hasListener)
		})
	}
}
