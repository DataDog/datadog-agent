// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMod(t *testing.T) {
	testInstances := []struct {
		name              string
		modPath           string
		expectedErr       error
		expectedGoVersion string
		expectedDepCount  int
	}{
		{
			name:              "Correct module",
			modPath:           "./testdata/match.go.mod",
			expectedErr:       nil,
			expectedGoVersion: "1.25.0",
			expectedDepCount:  1,
		},
		{
			name:              "Correct module with patch version in go directive",
			modPath:           "./testdata/patchgoversion.go.mod",
			expectedErr:       nil,
			expectedGoVersion: "1.25.6",
			expectedDepCount:  0,
		},
		{
			name:        "Missing module",
			modPath:     "./testdata/nonexistent",
			expectedErr: fmt.Errorf("could not read %s", filepath.FromSlash("testdata/nonexistent/go.mod")),
		},
		{
			name:        "Badly formatted module",
			modPath:     "./testdata/badformat.go.mod",
			expectedErr: fmt.Errorf("could not parse %s", filepath.FromSlash("testdata/badformat.go.mod")),
		},
	}

	for _, testInstance := range testInstances {
		t.Run(testInstance.name, func(t *testing.T) {
			parsedFile, err := parseMod(testInstance.modPath)
			if testInstance.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, err, testInstance.expectedErr)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, parsedFile.Go)
				assert.Equal(t, parsedFile.Go.Version, testInstance.expectedGoVersion)
				assert.Len(t, parsedFile.Require, testInstance.expectedDepCount)
			}
		})
	}
}

func TestFilterMatch(t *testing.T) {
	testInstances := []struct {
		name             string
		modPath          string
		expectedMatches  []string
		expectedDepCount int
	}{
		{
			name:             "With matches",
			modPath:          "./testdata/match.go.mod",
			expectedMatches:  []string{"github.com/DataDog/datadog-agent/pkg/test"},
			expectedDepCount: 1,
		},
		{
			name:             "Without matches",
			modPath:          "./testdata/nomatch.go.mod",
			expectedMatches:  []string{},
			expectedDepCount: 2,
		},
	}

	for _, testInstance := range testInstances {
		t.Run(testInstance.name, func(t *testing.T) {
			parsedFile, err := parseMod(testInstance.modPath)
			assert.NoError(t, err)
			require.NotNil(t, parsedFile.Go)
			assert.Equal(t, parsedFile.Go.Version, "1.25.0")
			assert.Len(t, parsedFile.Require, testInstance.expectedDepCount)

			matches := filter(parsedFile, "github.com/DataDog/datadog-agent")
			assert.ElementsMatch(t, matches, testInstance.expectedMatches)
		})
	}
}
