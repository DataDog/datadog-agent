// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tokens"
)

// TestTokenBasedMaskSequences tests the basic token-based masking functionality
func TestTokenBasedMaskSequences(t *testing.T) {
	rule := &config.ProcessingRule{
		Type:            config.MaskSequences,
		Name:            "test_ssn",
		TokenPatternStr: []string{"D3", "Dash", "D2", "Dash", "D4"},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.D3),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D2),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D4),
		},
		PrefilterKeywords:    []string{"-"},
		PrefilterKeywordsRaw: [][]byte{[]byte("-")},
		Placeholder:          []byte("[SSN_REDACTED]"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := message.NewMessageWithSource([]byte("SSN: 123-45-6789"), message.StatusInfo, source, 0)

	result := processor.applyRedactingRules(msg)

	assert.True(t, result)
	assert.Equal(t, []byte("SSN: [SSN_REDACTED]"), msg.GetContent())
}

// TestTokenBasedExcludeAtMatch tests exclude_at_match functionality
func TestTokenBasedExcludeAtMatch(t *testing.T) {
	rule := &config.ProcessingRule{
		Type:            config.ExcludeAtMatch,
		Name:            "test_exclude",
		TokenPatternStr: []string{"C6"}, // "health"
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C6),
		},
		PrefilterKeywords:    []string{"health"},
		PrefilterKeywordsRaw: [][]byte{[]byte("health")},
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := message.NewMessageWithSource([]byte("GET /health HTTP/1.1"), message.StatusInfo, source, 0)

	result := processor.applyRedactingRules(msg)

	assert.False(t, result) // Should be excluded
}

// TestTokenBasedIncludeAtMatch tests include_at_match functionality
func TestTokenBasedIncludeAtMatch(t *testing.T) {
	rule := &config.ProcessingRule{
		Type:            config.IncludeAtMatch,
		Name:            "test_include",
		TokenPatternStr: []string{"C5"}, // "ERROR"
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C5),
		},
		PrefilterKeywords:    []string{"ERROR"},
		PrefilterKeywordsRaw: [][]byte{[]byte("ERROR")},
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Should be included
	msg1 := message.NewMessageWithSource([]byte("ERROR: something went wrong"), message.StatusInfo, source, 0)
	result1 := processor.applyRedactingRules(msg1)
	assert.True(t, result1)

	// Should be excluded (doesn't match)
	msg2 := message.NewMessageWithSource([]byte("INFO: everything is fine"), message.StatusInfo, source, 0)
	result2 := processor.applyRedactingRules(msg2)
	assert.False(t, result2)
}

// TestPrefilterOptimization verifies prefilter keywords enable early exit
func TestPrefilterOptimization(t *testing.T) {
	rule := &config.ProcessingRule{
		Type:            config.MaskSequences,
		Name:            "test_prefilter",
		TokenPatternStr: []string{"D3", "Dash", "D2", "Dash", "D4"},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.D3),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D2),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D4),
		},
		PrefilterKeywords:    []string{"-"},
		PrefilterKeywordsRaw: [][]byte{[]byte("-")},
		Placeholder:          []byte("[REDACTED]"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Message without dash - should skip tokenization via prefilter
	msg := message.NewMessageWithSource([]byte("No SSN here just numbers 1234567890"), message.StatusInfo, source, 0)
	result := processor.applyRedactingRules(msg)

	assert.True(t, result)
	assert.Equal(t, []byte("No SSN here just numbers 1234567890"), msg.GetContent())
}

// TestPrefilterEarlyExit explicitly validates that tokenization is skipped
// when prefilter keywords don't match
func TestPrefilterEarlyExit(t *testing.T) {
	// Create a rule that would normally mask SSN patterns
	rule := &config.ProcessingRule{
		Type:            config.MaskSequences,
		Name:            "test_early_exit",
		TokenPatternStr: []string{"D3", "Dash", "D2", "Dash", "D4"},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.D3),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D2),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D4),
		},
		PrefilterKeywords:    []string{"-", "SSN"},
		PrefilterKeywordsRaw: [][]byte{[]byte("-"), []byte("SSN")},
		Placeholder:          []byte("[REDACTED]"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Test 1: Message missing BOTH keywords - prefilter should reject
	// Even though it has the pattern 123-45-6789, it should NOT be redacted
	msg1 := message.NewMessageWithSource(
		[]byte("ID: 123-45-6789 for user account"),
		message.StatusInfo, source, 0,
	)
	result1 := processor.applyRedactingRules(msg1)
	assert.True(t, result1, "Should process message (not exclude)")
	assert.Equal(t, []byte("ID: 123-45-6789 for user account"), msg1.GetContent(),
		"Should NOT redact - missing 'SSN' keyword")

	// Test 2: Message has dash but no "SSN" - should skip rule
	msg2 := message.NewMessageWithSource(
		[]byte("Phone: 123-45-6789 for contact"),
		message.StatusInfo, source, 0,
	)
	result2 := processor.applyRedactingRules(msg2)
	assert.True(t, result2, "Should process message")
	assert.Equal(t, []byte("Phone: 123-45-6789 for contact"), msg2.GetContent(),
		"Should NOT redact - missing 'SSN' keyword")

	// Test 3: Message has "SSN" but no dash - should skip rule
	msg3 := message.NewMessageWithSource(
		[]byte("SSN number is required for verification"),
		message.StatusInfo, source, 0,
	)
	result3 := processor.applyRedactingRules(msg3)
	assert.True(t, result3, "Should process message")
	assert.Equal(t, []byte("SSN number is required for verification"), msg3.GetContent(),
		"Should NOT redact - missing dash character")

	// Test 4: Message has BOTH keywords - prefilter passes, pattern matches, SHOULD redact
	msg4 := message.NewMessageWithSource(
		[]byte("SSN: 123-45-6789 on file"),
		message.StatusInfo, source, 0,
	)
	result4 := processor.applyRedactingRules(msg4)
	assert.True(t, result4, "Should process message")
	assert.Equal(t, []byte("SSN: [REDACTED] on file"), msg4.GetContent(),
		"SHOULD redact - has both keywords and matches pattern")
}

// TestPrefilterAllKeywordsRequired verifies that ALL prefilter keywords must be present
func TestPrefilterAllKeywordsRequired(t *testing.T) {
	rule := &config.ProcessingRule{
		Type:                 config.ExcludeAtMatch,
		Name:                 "test_all_keywords",
		TokenPatternStr:      []string{"C5"}, // "error"
		TokenPattern:         []tokens.Token{tokens.NewSimpleToken(tokens.C5)},
		PrefilterKeywords:    []string{"error", "critical", "failure"},
		PrefilterKeywordsRaw: [][]byte{[]byte("error"), []byte("critical"), []byte("failure")},
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Has "error" and "critical" but missing "failure" - should NOT match
	msg1 := message.NewMessageWithSource(
		[]byte("error: critical system issue detected"),
		message.StatusInfo, source, 0,
	)
	result1 := processor.applyRedactingRules(msg1)
	assert.True(t, result1, "Should NOT exclude - missing 'failure' keyword")

	// Has all three keywords - should proceed to tokenization and match
	msg2 := message.NewMessageWithSource(
		[]byte("error: critical failure in system"),
		message.StatusInfo, source, 0,
	)
	result2 := processor.applyRedactingRules(msg2)
	assert.False(t, result2, "Should exclude - has all keywords and matches pattern")
}

// TestTokenExcludeTruncated tests the exclude_truncated rule type
func TestTokenExcludeTruncated(t *testing.T) {
	rule := &config.ProcessingRule{
		Type: config.ExcludeTruncated,
		Name: "test_truncated",
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := message.NewMessageWithSource([]byte("Some message"), message.StatusInfo, source, 0)
	msg.ParsingExtra.IsTruncated = true

	result := processor.applyRedactingRules(msg)

	assert.False(t, result) // Should be excluded
}

// TestTokenLengthConstraints tests variable-length token patterns (e.g., IPv4)
func TestTokenLengthConstraints(t *testing.T) {
	rule := &config.ProcessingRule{
		Type:            config.MaskSequences,
		Name:            "test_ipv4",
		TokenPatternStr: []string{"DAny", "Period", "DAny", "Period", "DAny", "Period", "DAny"},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.DAny), // Wildcard: matches any D1-D10
			tokens.NewSimpleToken(tokens.Period),
			tokens.NewSimpleToken(tokens.DAny),
			tokens.NewSimpleToken(tokens.Period),
			tokens.NewSimpleToken(tokens.DAny),
			tokens.NewSimpleToken(tokens.Period),
			tokens.NewSimpleToken(tokens.DAny),
		},
		LengthConstraints: []config.LengthConstraint{
			{TokenIndex: 0, MinLength: 1, MaxLength: 3}, // First octet
			{TokenIndex: 2, MinLength: 1, MaxLength: 3}, // Second octet
			{TokenIndex: 4, MinLength: 1, MaxLength: 3}, // Third octet
			{TokenIndex: 6, MinLength: 1, MaxLength: 3}, // Fourth octet
		},
		PrefilterKeywords:    []string{"."},
		PrefilterKeywordsRaw: [][]byte{[]byte(".")},
		Placeholder:          []byte("[IP_REDACTED]"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Valid IPv4: 192.168.1.100
	msg1 := message.NewMessageWithSource([]byte("Server IP: 192.168.1.100"), message.StatusInfo, source, 0)
	result1 := processor.applyRedactingRules(msg1)
	assert.True(t, result1)
	assert.Equal(t, []byte("Server IP: [IP_REDACTED]"), msg1.GetContent())

	// Valid IPv4: 1.2.3.4
	msg2 := message.NewMessageWithSource([]byte("Server IP: 1.2.3.4"), message.StatusInfo, source, 0)
	result2 := processor.applyRedactingRules(msg2)
	assert.True(t, result2)
	assert.Equal(t, []byte("Server IP: [IP_REDACTED]"), msg2.GetContent())

	// Invalid IPv4: 1234.1.1.1 (first octet too long)
	msg3 := message.NewMessageWithSource([]byte("Server IP: 1234.1.1.1"), message.StatusInfo, source, 0)
	result3 := processor.applyRedactingRules(msg3)
	assert.True(t, result3)
	assert.Equal(t, []byte("Server IP: 1234.1.1.1"), msg3.GetContent()) // Not redacted

	// No IP address
	msg4 := message.NewMessageWithSource([]byte("No IP here"), message.StatusInfo, source, 0)
	result4 := processor.applyRedactingRules(msg4)
	assert.True(t, result4)
	assert.Equal(t, []byte("No IP here"), msg4.GetContent())
}
