// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMod(t *testing.T) {
	testInstances := []struct {
		name        string
		modPath     string
		expectedErr error
	}{
		{
			name:        "Correct module",
			modPath:     "./testdata/match",
			expectedErr: nil,
		},
		{
			name:        "Correct module with slash",
			modPath:     "./testdata/match/",
			expectedErr: nil,
		},
		{
			name:        "Missing module",
			modPath:     "./testdata/nonexistant/",
			expectedErr: fmt.Errorf("could not read go.mod file in ./testdata/nonexistant/"),
		},
		{
			name:        "Badly formatted module",
			modPath:     "./testdata/badformat/",
			expectedErr: fmt.Errorf("could not parse go.mod file in ./testdata/badformat/"),
		},
	}

	for _, testInstance := range testInstances {
		t.Run(testInstance.name, func(t *testing.T) {
			_, err := parseMod(testInstance.modPath)
			if testInstance.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, err, testInstance.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFilterMatch(t *testing.T) {
	testInstances := []struct {
		name            string
		modPath         string
		expectedMatches []string
	}{
		{
			name:            "With matches",
			modPath:         "./testdata/match",
			expectedMatches: []string{"github.com/DataDog/datadog-agent/pkg/test"},
		},
		{
			name:            "Without matches",
			modPath:         "./testdata/nomatch",
			expectedMatches: []string{},
		},
	}

	for _, testInstance := range testInstances {
		t.Run(testInstance.name, func(t *testing.T) {
			parsedFile, err := parseMod(testInstance.modPath)
			assert.NoError(t, err)

			matches := filter(parsedFile, "github.com/DataDog/datadog-agent")
			assert.ElementsMatch(t, matches, testInstance.expectedMatches)
		})
	}
}
