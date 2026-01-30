// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package main

import (
	"errors"
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
			modPath:           "./testdata/match",
			expectedErr:       nil,
			expectedGoVersion: "1.25.0",
			expectedDepCount:  1,
		},
		{
			name:              "Correct module with slash",
			modPath:           "./testdata/match/",
			expectedErr:       nil,
			expectedGoVersion: "1.25.0",
			expectedDepCount:  1,
		},
		{
			name:              "Correct module with patch version in go directive",
			modPath:           "./testdata/patchgoversion/",
			expectedErr:       nil,
			expectedGoVersion: "1.25.6",
			expectedDepCount:  0,
		},
		{
			name:        "Missing module",
			modPath:     "./testdata/nonexistant/",
			expectedErr: errors.New("could not read go.mod file in ./testdata/nonexistant/"),
		},
		{
			name:        "Badly formatted module",
			modPath:     "./testdata/badformat/",
			expectedErr: errors.New("could not parse go.mod file in ./testdata/badformat/"),
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
			modPath:          "./testdata/match",
			expectedMatches:  []string{"github.com/DataDog/datadog-agent/pkg/test"},
			expectedDepCount: 1,
		},
		{
			name:             "Without matches",
			modPath:          "./testdata/nomatch",
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
