// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Test-only helper functions

func TestNewPattern(t *testing.T) {
	// Create a simple token list
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	patternID := uint64(12345)
	pattern := newPattern(tl, patternID)

	assert.NotNil(t, pattern)
	assert.Equal(t, patternID, pattern.PatternID)
	assert.Equal(t, tl, pattern.Template, "Template should be the initial token list")
	assert.Equal(t, tl, pattern.Sample, "Sample should be the initial token list")
	assert.Equal(t, 1, pattern.LogCount, "LogCount should be 1 for first log")
	assert.Equal(t, 0, len(pattern.Positions), "No wildcards initially")
	assert.False(t, pattern.CreatedAt.IsZero(), "CreatedAt should be set")
	assert.False(t, pattern.UpdatedAt.IsZero(), "UpdatedAt should be set")
}

func TestAddTokenList(t *testing.T) {
	// Note: addTokenList() was inlined into Cluster.AddTokenListToPatterns()
	// This test now verifies that LogCount and UpdatedAt can be modified directly
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	initialLogCount := pattern.LogCount
	initialUpdatedAt := pattern.UpdatedAt

	// Simulate what cluster does when adding to existing pattern
	time.Sleep(1 * time.Millisecond) // Ensure time difference
	pattern.LogCount++
	pattern.UpdatedAt = time.Now()

	assert.Equal(t, initialLogCount+1, pattern.LogCount, "LogCount should increment")
	assert.True(t, pattern.UpdatedAt.After(initialUpdatedAt), "UpdatedAt should be updated")
}

func TestSize(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	assert.Equal(t, 1.0, pattern.GetFrequency())

	// Simulate adding more logs (what cluster does)
	pattern.LogCount++
	assert.Equal(t, 2.0, pattern.GetFrequency())

	pattern.LogCount++
	assert.Equal(t, 3.0, pattern.GetFrequency())
}

func TestGetPatternString_NoWildcards(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	result := pattern.GetPatternString()

	assert.Equal(t, "Service started", result)
}

func TestGetPatternString_WithWildcards(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))

	pattern := newPattern(tl, 12345)
	pattern.Positions = []int{2}
	result := pattern.GetPatternString()

	// Wildcard tokens are omitted from the template
	assert.Equal(t, "Service ", result)
}

func TestGetPatternString_NilTemplate(t *testing.T) {
	pattern := &Pattern{
		Template: nil,
	}
	result := pattern.GetPatternString()

	assert.Equal(t, "", result)
}

func TestHasWildcards(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)

	// No wildcards initially
	assert.False(t, pattern.hasWildcards())

	// Add wildcard positions
	pattern.Positions = []int{1, 3}
	assert.True(t, pattern.hasWildcards())
}

func TestGetWildcardPositions(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	pattern.Positions = []int{1, 3, 5}

	assert.Equal(t, []int{1, 3, 5}, pattern.Positions)
}

// getParamCount returns the number of parameters/wildcards in a pattern.
func getParamCount(p *Pattern) int {
	return len(p.Positions)
}

func TestGetParamCount(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)

	// No wildcards
	assert.Equal(t, 0, getParamCount(pattern))

	// Add wildcard positions
	pattern.Positions = []int{1, 3, 5}
	assert.Equal(t, 3, getParamCount(pattern))
}

func TestGetWildcardCharPositions(t *testing.T) {
	// Create pattern: "Service " (wildcard omitted from template)
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))

	pattern := newPattern(tl, 12345)
	pattern.Positions = []int{2}

	charPositions := pattern.GetWildcardCharPositions()
	// "Service " = 8 chars, wildcard injection point is at position 8
	assert.Equal(t, []int{8}, charPositions)
}

func TestGetWildcardCharPositions_MultipleWildcards(t *testing.T) {
	// Create pattern: "Error  in " (both wildcards omitted from template)
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Error", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "code", token.IsWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "in", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "module", token.IsWildcard))

	pattern := newPattern(tl, 12345)
	pattern.Positions = []int{2, 6}

	charPositions := pattern.GetWildcardCharPositions()
	// Template is "Error  in " (wildcards omitted): "Error " (6 chars) + " in " (4 chars) = 10 chars
	// First wildcard injection at position 6 (after "Error ")
	// Second wildcard injection at position 10 (after "Error  in ")
	assert.Equal(t, []int{6, 10}, charPositions)
}

func TestGetWildcardCharPositions_NilTemplate(t *testing.T) {
	pattern := &Pattern{
		Template: nil,
	}

	charPositions := pattern.GetWildcardCharPositions()
	assert.Nil(t, charPositions)
}

func TestGetWildcardValues(t *testing.T) {
	// Create sample log: "Service started"
	sample := token.NewTokenList()
	sample.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	sample.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	sample.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	// Create template with wildcard: "Service *"
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))

	pattern := newPattern(sample, 12345)
	pattern.Template = tl
	pattern.Positions = []int{2}

	values := pattern.GetWildcardValues(sample)
	assert.Equal(t, []string{"started"}, values)
}

func TestGetWildcardValues_NilTemplate(t *testing.T) {
	sample := token.NewTokenList()
	sample.Add(token.NewToken(token.TokenWord, "Test", token.PotentialWildcard))

	pattern := newPattern(sample, 12345)
	pattern.Template = nil

	values := pattern.GetWildcardValues(sample)
	assert.Empty(t, values)
}

func TestGetWildcardValues_NilSample(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.IsWildcard))

	pattern := newPattern(tl, 12345)
	pattern.Sample = nil
	pattern.Positions = []int{0}

	// Test with the template itself since sample is nil
	values := pattern.GetWildcardValues(tl)
	assert.Equal(t, []string{"Test"}, values)
}

func TestExtractWildcardValues(t *testing.T) {
	// Create template: "Service *"
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))

	pattern := newPattern(template, 12345)
	pattern.Template = template
	pattern.Positions = []int{2}

	// Create incoming log: "Service crashed"
	incoming := token.NewTokenList()
	incoming.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, "crashed", token.PotentialWildcard))

	values := pattern.GetWildcardValues(incoming)
	assert.Equal(t, []string{"crashed"}, values)
}

func TestExtractWildcardValues_MultipleWildcards(t *testing.T) {
	// Create template: "* in * at *"
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "value1", token.IsWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "in", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value2", token.IsWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "at", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value3", token.IsWildcard))

	pattern := newPattern(template, 12345)
	pattern.Template = template
	pattern.Positions = []int{0, 4, 8}

	// Create incoming log: "Error in module at line"
	incoming := token.NewTokenList()
	incoming.Add(token.NewToken(token.TokenWord, "Error", token.PotentialWildcard))
	incoming.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, "in", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, "module", token.PotentialWildcard))
	incoming.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, "at", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	incoming.Add(token.NewToken(token.TokenWord, "line", token.PotentialWildcard))

	values := pattern.GetWildcardValues(incoming)
	assert.Equal(t, []string{"Error", "module", "line"}, values)
}

func TestExtractWildcardValues_NilTemplate(t *testing.T) {
	pattern := &Pattern{
		Template:  nil,
		Positions: []int{0},
	}

	incoming := token.NewTokenList()
	incoming.Add(token.NewToken(token.TokenWord, "Test", token.PotentialWildcard))

	values := pattern.GetWildcardValues(incoming)
	assert.Equal(t, []string{}, values)
}

func TestExtractWildcardValues_NoPositions(t *testing.T) {
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	pattern := newPattern(template, 12345)
	pattern.Positions = []int{} // No wildcards

	incoming := token.NewTokenList()
	incoming.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	values := pattern.GetWildcardValues(incoming)
	assert.Equal(t, []string{}, values)
}

func TestExtractWildcardValues_PositionOutOfBounds(t *testing.T) {
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "Test", token.IsWildcard))

	pattern := newPattern(template, 12345)
	pattern.Positions = []int{0, 5} // Position 5 is out of bounds

	incoming := token.NewTokenList()
	incoming.Add(token.NewToken(token.TokenWord, "Value", token.PotentialWildcard))

	values := pattern.GetWildcardValues(incoming)
	// CRITICAL: Must return same length as Positions to match ParamCount
	// Out-of-bounds positions are filled with empty strings
	assert.Equal(t, []string{"Value", ""}, values, "Should maintain Positions length with empty strings for out-of-bounds")
}

func TestSanitizeForTemplate_PrintableChars(t *testing.T) {
	input := "Hello World 123"
	result := sanitizeForTemplate(input)
	assert.Equal(t, "Hello World 123", result)
}

func TestSanitizeForTemplate_NonPrintableChars(t *testing.T) {
	// Include null byte, bell, backspace
	input := "Hello\x00\x07\x08World"
	result := sanitizeForTemplate(input)
	assert.Equal(t, "HelloWorld", result, "Non-printable characters should be removed")
}

func TestSanitizeForTemplate_DELCharacter(t *testing.T) {
	input := "Hello\x7FWorld"
	result := sanitizeForTemplate(input)
	assert.Equal(t, "HelloWorld", result, "DEL character should be removed")
}

func TestSanitizeForTemplate_SpecialChars(t *testing.T) {
	input := "Service: Error! @user #tag"
	result := sanitizeForTemplate(input)
	assert.Equal(t, "Service: Error! @user #tag", result, "Special chars should be kept")
}

func TestSanitizeForTemplate_EmptyString(t *testing.T) {
	input := ""
	result := sanitizeForTemplate(input)
	assert.Equal(t, "", result)
}

func TestSanitizeForTemplate_UnicodeChars(t *testing.T) {
	input := "Hello 世界 🌍"
	result := sanitizeForTemplate(input)
	// Emoji (🌍) is above 0xFFFD and gets filtered out by sanitizeForTemplate
	// CJK characters (世界) are within the acceptable range
	assert.Equal(t, "Hello 世界 ", result, "CJK chars preserved, emoji filtered")
}

func TestPattern_IntegrationScenario(t *testing.T) {
	// Simulate a realistic pattern lifecycle

	// 1. First log arrives
	log1 := token.NewTokenList()
	log1.Add(token.NewToken(token.TokenWord, "ERROR", token.NotWildcard))
	log1.Add(token.NewToken(token.TokenWord, ":", token.NotWildcard))
	log1.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	log1.Add(token.NewToken(token.TokenWord, "Database", token.PotentialWildcard))
	log1.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	log1.Add(token.NewToken(token.TokenWord, "connection", token.PotentialWildcard))
	log1.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	log1.Add(token.NewToken(token.TokenWord, "failed", token.PotentialWildcard))

	pattern := newPattern(log1, 9999)

	assert.Equal(t, 1, pattern.LogCount)
	assert.False(t, pattern.hasWildcards())
	assert.Equal(t, "ERROR: Database connection failed", pattern.GetPatternString())

	// 2. Pattern updated with wildcards (simulated)
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "ERROR", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, ":", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))

	pattern.Template = template
	pattern.Positions = []int{3, 5, 7}
	pattern.LogCount++ // Simulate second log being added
	pattern.UpdatedAt = time.Now()

	assert.Equal(t, 2, pattern.LogCount)
	assert.True(t, pattern.hasWildcards())
	assert.Equal(t, 3, getParamCount(pattern))
	// Wildcard tokens are omitted from template, leaving: "ERROR: " + " " + " " = "ERROR:   "
	assert.Equal(t, "ERROR:   ", pattern.GetPatternString())

	// 3. Extract wildcard values from new log
	log2 := token.NewTokenList()
	log2.Add(token.NewToken(token.TokenWord, "ERROR", token.NotWildcard))
	log2.Add(token.NewToken(token.TokenWord, ":", token.NotWildcard))
	log2.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	log2.Add(token.NewToken(token.TokenWord, "Network", token.PotentialWildcard))
	log2.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	log2.Add(token.NewToken(token.TokenWord, "timeout", token.PotentialWildcard))
	log2.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	log2.Add(token.NewToken(token.TokenWord, "reached", token.PotentialWildcard))

	values := pattern.GetWildcardValues(log2)
	assert.Equal(t, []string{"Network", "timeout", "reached"}, values)
}
