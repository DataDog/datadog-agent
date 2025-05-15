// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"maps"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// TestPopulateCapabilities tests the populateCapabilities function with different symbol configurations
func TestPopulateCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		setupSymbols      func() map[string]struct{}
		expectInitErr     bool
		expectedLookupErr error
		testSymbol        string
	}{
		{
			name: "AllSymbolsAvailable",
			setupSymbols: func() map[string]struct{} {
				return maps.Clone(allSymbols)
			},
			expectInitErr:     false,
			expectedLookupErr: nil,
			testSymbol:        toNativeName("GetUUID"),
		},
		{
			name: "MissingCriticalSymbol",
			setupSymbols: func() map[string]struct{} {
				// Make a copy of all symbols
				symbols := maps.Clone(allSymbols)

				// Remove a critical symbol - nvmlDeviceGetCount
				delete(symbols, toNativeName("GetCount"))

				return symbols
			},
			expectInitErr:     true,
			expectedLookupErr: NewNvmlAPIErrorOrNil(toNativeName("GetCount"), nvml.ERROR_FUNCTION_NOT_FOUND),
			testSymbol:        toNativeName("GetCount"),
		},
		{
			name: "MissingNonCriticalSymbol",
			setupSymbols: func() map[string]struct{} {
				// Make a copy of all symbols
				symbols := maps.Clone(allSymbols)

				// Remove a non-critical symbol - nvmlSystemGetDriverVersion
				delete(symbols, "nvmlSystemGetDriverVersion")

				return symbols
			},
			expectInitErr:     false,
			expectedLookupErr: NewNvmlAPIErrorOrNil("nvmlSystemGetDriverVersion", nvml.ERROR_FUNCTION_NOT_FOUND),
			testSymbol:        "nvmlSystemGetDriverVersion",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new safeNvml instance
			safenvml := &safeNvml{}

			// Create a mock with the available symbols for this test
			availableSymbols := tc.setupSymbols()
			// Create mock with all symbols available
			mockNvml := testutil.GetBasicNvmlMockWithOptions(
				testutil.WithSymbolsMock(availableSymbols),
			)
			WithPartialMockNVML(t, mockNvml, availableSymbols)

			// Set the library instance directly to bypass initialization
			safenvml.lib = mockNvml

			// Call populateCapabilities
			err := safenvml.populateCapabilities()

			if tc.expectInitErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Test lookup for specific symbols
			err = safenvml.lookup(tc.testSymbol)
			require.Equal(t, tc.expectedLookupErr, err)
		})
	}
}

// Tests for the NvmlAPIError type
func TestNvmlAPIError(t *testing.T) {
	// Define test cases with different error codes and API names
	tests := []struct {
		name      string
		apiName   string
		errorCode nvml.Return
	}{
		{
			name:      "symbol not found error",
			apiName:   "TestSymbol",
			errorCode: nvml.ERROR_FUNCTION_NOT_FOUND,
		},
		{
			name:      "not supported error",
			apiName:   "TestAPI",
			errorCode: nvml.ERROR_NOT_SUPPORTED,
		},
		{
			name:      "other NVML error",
			apiName:   "TestAPI",
			errorCode: nvml.ERROR_UNKNOWN,
		},
		{
			name:      "wrapped error",
			apiName:   "TestSymbol",
			errorCode: nvml.ERROR_FUNCTION_NOT_FOUND,
		},
	}

	// Run subtests for each error type
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create the NvmlAPIError
			err := NewNvmlAPIErrorOrNil(tc.apiName, tc.errorCode)
			require.NotNil(t, err, "Expected non-nil error for non-SUCCESS error code")

			var nvmlErr *NvmlAPIError
			require.True(t, errors.As(err, &nvmlErr))
			require.Equal(t, tc.apiName, nvmlErr.APIName)
			require.Equal(t, tc.errorCode, nvmlErr.NvmlErrorCode)
		})
	}

	// Special test for SUCCESS which should return nil
	t.Run("SUCCESS returns nil", func(t *testing.T) {
		err := NewNvmlAPIErrorOrNil("TestAPI", nvml.SUCCESS)
		require.Nil(t, err)
	})
}
