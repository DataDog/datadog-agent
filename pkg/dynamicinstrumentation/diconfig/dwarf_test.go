// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	"github.com/stretchr/testify/require"
)

func TestLoadFunctionDefinitions(t *testing.T) {
	testCases := []struct {
		name            string
		binaryPath      string
		targetFuncNames []string
		expectError     bool
		postAssertions  func(t *testing.T, typeMap *ditypes.TypeMap)
	}{
		{
			name:            "Basic Load",
			binaryPath:      filepath.Join("testdata", "ref-table-edge.debug"),
			targetFuncNames: []string{"main.(*Server).HandleCreateSync"},
			expectError:     false,
			postAssertions: func(t *testing.T, typeMap *ditypes.TypeMap) {
				fmt.Println("typeMap.Functions", typeMap.Functions)
				require.NotNil(t, typeMap, "TypeMap should not be nil")
				for i := range typeMap.Functions {
					fmt.Println(">", i)
				}
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			targetFuncsMap := make(map[string]bool, len(tc.targetFuncNames))
			for _, name := range tc.targetFuncNames {
				targetFuncsMap[name] = false
			}

			// Open the ELF file
			elfFile, err := safeelf.Open(tc.binaryPath)
			require.NoError(t, err, "Failed to open ELF file %s", tc.binaryPath)
			defer elfFile.Close()

			// Get DWARF data
			dwarfData, err := elfFile.DWARF()
			require.NoError(t, err, "Failed to get DWARF data from ELF file")
			require.NotNil(t, dwarfData, "DWARF data should not be nil")

			// Call the function under test
			typeMap, err := loadFunctionDefinitions(dwarfData, targetFuncsMap)

			// Basic assertions
			if tc.expectError {
				require.Error(t, err, "Expected an error from loadFunctionDefinitions")
			} else {
				require.NoError(t, err, "loadFunctionDefinitions returned an unexpected error")
			}

			// Run specific assertions for this test case if provided
			if tc.postAssertions != nil {
				tc.postAssertions(t, typeMap)
			}
		})
	}
}
