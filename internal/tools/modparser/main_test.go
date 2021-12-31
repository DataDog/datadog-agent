package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMod(t *testing.T) {
	testInstances := []struct {
		modPath     string
		expectedErr error
	}{
		{
			modPath:     "./testdata/match",
			expectedErr: nil,
		},
		{
			modPath:     "./testdata/match/",
			expectedErr: nil,
		},
		{
			modPath:     "./testdata/nonexistant/",
			expectedErr: fmt.Errorf("could not read go.mod file in ./testdata/nonexistant/"),
		},
		{
			modPath:     "./testdata/badformat/",
			expectedErr: fmt.Errorf("could not parse go.mod file in ./testdata/badformat/"),
		},
	}

	for _, testInstance := range testInstances {
		_, err := parseMod(testInstance.modPath)
		if testInstance.expectedErr != nil {
			assert.Error(t, err)
			assert.Equal(t, err, testInstance.expectedErr)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestFilterMatch(t *testing.T) {
	testInstances := []struct {
		modPath         string
		expectedMatches []string
	}{
		{
			modPath:         "./testdata/match",
			expectedMatches: []string{"github.com/DataDog/datadog-agent/pkg/test"},
		},
		{
			modPath:         "./testdata/nomatch",
			expectedMatches: []string{},
		},
	}

	for _, testInstance := range testInstances {
		parsedFile, err := parseMod(testInstance.modPath)
		assert.NoError(t, err)

		matches := filter(parsedFile, "github.com/DataDog/datadog-agent")
		assert.ElementsMatch(t, matches, testInstance.expectedMatches)
	}
}
