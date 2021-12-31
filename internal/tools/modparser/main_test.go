package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMod(t *testing.T) {
	modPath := "./testdata/match"

	_, err := parseMod(modPath)
	assert.NoError(t, err)
}

func TestParseModSlash(t *testing.T) {
	modPath := "./testdata/match/"

	_, err := parseMod(modPath)
	assert.NoError(t, err)
}

func TestParseModMissing(t *testing.T) {
	modPath := "./testdata/nonexistant/"

	_, err := parseMod(modPath)
	assert.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("could not read go.mod file in %s", modPath))
}

func TestParseModBadFormat(t *testing.T) {
	modPath := "./testdata/badformat/"

	_, err := parseMod(modPath)
	assert.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("could not parse go.mod file in %s", modPath))
}

func TestFilterMatch(t *testing.T) {
	modPath := "./testdata/match"

	parsedFile, err := parseMod(modPath)
	assert.NoError(t, err)

	matches := filter(parsedFile, "github.com/DataDog/datadog-agent")
	expectedMatches := []string{"github.com/DataDog/datadog-agent/pkg/test"}
	assert.Equal(t, 1, len(matches))
	assert.ElementsMatch(t, expectedMatches, matches)
}

func TestFilterNoMatch(t *testing.T) {
	modPath := "./testdata/nomatch"

	parsedFile, err := parseMod(modPath)
	assert.NoError(t, err)

	matches := filter(parsedFile, "github.com/DataDog/datadog-agent")
	assert.Equal(t, 0, len(matches))
}
