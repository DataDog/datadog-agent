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

// TestExcludeTranscriptWarning tests excluding transcript insight warnings
func TestExcludeTranscriptWarning(t *testing.T) {
	rule := &config.ProcessingRule{
		Type: config.ExcludeAtMatch,
		Name: "exclude_transcript_insights_warning",
		TokenPatternStr: []string{
			"C7", "Colon", "C4", "Colon", "C10", "Space",
			"C8", "Space", "C6", "Space", "C2",
		},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C7), // WARNING
			tokens.NewSimpleToken(tokens.Colon),
			tokens.NewSimpleToken(tokens.C4), // root
			tokens.NewSimpleToken(tokens.Colon),
			tokens.NewSimpleToken(tokens.C10), // Transcript
			tokens.NewSimpleToken(tokens.Space),
			tokens.NewSimpleToken(tokens.C8), // insights
			tokens.NewSimpleToken(tokens.Space),
			tokens.NewSimpleToken(tokens.C6), // result
			tokens.NewSimpleToken(tokens.Space),
			tokens.NewSimpleToken(tokens.C2), // is
		},
		PrefilterKeywords:    []string{"WARNING", "Transcript", "insights"},
		PrefilterKeywordsRaw: [][]byte{[]byte("WARNING"), []byte("Transcript"), []byte("insights")},
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Should be excluded
	msg1 := message.NewMessageWithSource(
		[]byte("WARNING:root:Transcript insights result is None due to error"),
		message.StatusInfo, source, 0,
	)
	result1 := processor.applyRedactingRules(msg1)
	assert.False(t, result1, "Should exclude transcript warning")

	// Should not be excluded (missing keyword)
	msg2 := message.NewMessageWithSource(
		[]byte("INFO:root:Transcript insights result is valid"),
		message.StatusInfo, source, 0,
	)
	result2 := processor.applyRedactingRules(msg2)
	assert.True(t, result2, "Should not exclude INFO message")
}

// TestExcludeWordTimestamp tests excluding word timestamp integrity messages
func TestExcludeWordTimestamp(t *testing.T) {
	// For "contains" patterns, we can use prefilter + simple token matching
	rule := &config.ProcessingRule{
		Type: config.ExcludeAtMatch,
		Name: "exclude_word_timestamp_integrity",
		TokenPatternStr: []string{
			"C4", "Space", "C9", "Space", "C9", "Space",
			"C6", "Comma",
		},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C4), // Word
			tokens.NewSimpleToken(tokens.Space),
			tokens.NewSimpleToken(tokens.C9), // timestamp
			tokens.NewSimpleToken(tokens.Space),
			tokens.NewSimpleToken(tokens.C9), // integrity
			tokens.NewSimpleToken(tokens.Space),
			tokens.NewSimpleToken(tokens.C6), // failed
			tokens.NewSimpleToken(tokens.Comma),
		},
		PrefilterKeywords:    []string{"Word timestamp integrity failed"},
		PrefilterKeywordsRaw: [][]byte{[]byte("Word timestamp integrity failed")},
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Should be excluded
	msg1 := message.NewMessageWithSource(
		[]byte("ERROR: Word timestamp integrity failed, adjusting based on heuristics"),
		message.StatusInfo, source, 0,
	)
	result1 := processor.applyRedactingRules(msg1)
	assert.False(t, result1, "Should exclude word timestamp message")

	// Should not be excluded
	msg2 := message.NewMessageWithSource(
		[]byte("Word timestamp validation passed"),
		message.StatusInfo, source, 0,
	)
	result2 := processor.applyRedactingRules(msg2)
	assert.True(t, result2, "Should not exclude validation passed message")
}

// TestMaskSimpleAPIKey tests masking simple API keys (letters/digits separate)
// Note: Full hex key masking (mixed letters/digits) is complex with current tokenizer
func TestMaskSimpleAPIKey(t *testing.T) {
	// Simplified version: api_key=<long string>
	// Pattern: api_key= followed by mixed char/digit tokens
	rule := &config.ProcessingRule{
		Type: config.MaskSequences,
		Name: "mask_api_keys_simple",
		TokenPatternStr: []string{
			"C3", "Underscore", "C3", "Equal", "CAny",
		},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C3), // api
			tokens.NewSimpleToken(tokens.Underscore),
			tokens.NewSimpleToken(tokens.C3), // key
			tokens.NewSimpleToken(tokens.Equal),
			tokens.NewSimpleToken(tokens.CAny), // Any char sequence
		},
		LengthConstraints: []config.LengthConstraint{
			{TokenIndex: 4, MinLength: 20, MaxLength: 64}, // Reasonable key length
		},
		PrefilterKeywords:    []string{"api_key="},
		PrefilterKeywordsRaw: [][]byte{[]byte("api_key=")},
		Placeholder:          []byte("api_key=**************************"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// API key with only letters (simpler case)
	msg1 := message.NewMessageWithSource(
		[]byte("config: api_key=abcdefghijklmnopqrstuvwxyz and more"),
		message.StatusInfo, source, 0,
	)
	result1 := processor.applyRedactingRules(msg1)
	assert.True(t, result1)
	assert.Equal(t, []byte("config: api_key=************************** and more"), msg1.GetContent())
}

// TestMaskDatadogAPIKey tests masking dd-api-key prefix pattern
func TestMaskDatadogAPIKey(t *testing.T) {
	rule := &config.ProcessingRule{
		Type: config.MaskSequences,
		Name: "mask_dd_api_keys",
		TokenPatternStr: []string{
			"C2", "Dash", "C3", "Dash", "C3", "Equal", "CAny",
		},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C2), // dd
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.C3), // api
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.C3), // key
			tokens.NewSimpleToken(tokens.Equal),
			tokens.NewSimpleToken(tokens.CAny), // Key value
		},
		LengthConstraints: []config.LengthConstraint{
			{TokenIndex: 6, MinLength: 20, MaxLength: 64},
		},
		PrefilterKeywords:    []string{"dd-api-key="},
		PrefilterKeywordsRaw: [][]byte{[]byte("dd-api-key=")},
		Placeholder:          []byte("dd-api-key=**************************"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{rule},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	msg := message.NewMessageWithSource(
		[]byte("Authorization: dd-api-key=abcdefghijklmnopqrstuvwxyz"),
		message.StatusInfo, source, 0,
	)
	result := processor.applyRedactingRules(msg)
	assert.True(t, result)
	assert.Equal(t, []byte("Authorization: dd-api-key=**************************"), msg.GetContent())
}

// TestMaskApplicationKey tests masking app_key or application_key patterns
func TestMaskApplicationKey(t *testing.T) {
	// Pattern for "app_key=<value>"
	ruleAppKey := &config.ProcessingRule{
		Type: config.MaskSequences,
		Name: "mask_app_keys",
		TokenPatternStr: []string{
			"C3", "Underscore", "C3", "Equal", "CAny",
		},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C3), // app
			tokens.NewSimpleToken(tokens.Underscore),
			tokens.NewSimpleToken(tokens.C3), // key
			tokens.NewSimpleToken(tokens.Equal),
			tokens.NewSimpleToken(tokens.CAny),
		},
		LengthConstraints: []config.LengthConstraint{
			{TokenIndex: 4, MinLength: 20, MaxLength: 64},
		},
		PrefilterKeywords:    []string{"app_key="},
		PrefilterKeywordsRaw: [][]byte{[]byte("app_key=")},
		Placeholder:          []byte("app_key=**********************************"),
	}

	// Pattern for "application_key=<value>"
	ruleApplicationKey := &config.ProcessingRule{
		Type: config.MaskSequences,
		Name: "mask_application_keys",
		TokenPatternStr: []string{
			"C10", "Underscore", "C3", "Equal", "CAny",
		},
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.C10), // application
			tokens.NewSimpleToken(tokens.Underscore),
			tokens.NewSimpleToken(tokens.C3), // key
			tokens.NewSimpleToken(tokens.Equal),
			tokens.NewSimpleToken(tokens.CAny),
		},
		LengthConstraints: []config.LengthConstraint{
			{TokenIndex: 4, MinLength: 20, MaxLength: 64},
		},
		PrefilterKeywords:    []string{"application_key="},
		PrefilterKeywordsRaw: [][]byte{[]byte("application_key=")},
		Placeholder:          []byte("application_key=**********************************"),
	}

	processor := &Processor{
		processingRules:       []*config.ProcessingRule{ruleAppKey, ruleApplicationKey},
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Test app_key
	msg1 := message.NewMessageWithSource(
		[]byte("config: app_key=abcdefghijklmnopqrstuvwxyz"),
		message.StatusInfo, source, 0,
	)
	result1 := processor.applyRedactingRules(msg1)
	assert.True(t, result1)
	assert.Equal(t, []byte("config: app_key=**********************************"), msg1.GetContent())

	// Test application_key
	msg2 := message.NewMessageWithSource(
		[]byte("config: application_key=abcdefghijklmnopqrstuvwxyz"),
		message.StatusInfo, source, 0,
	)
	result2 := processor.applyRedactingRules(msg2)
	assert.True(t, result2)
	assert.Equal(t, []byte("config: application_key=**********************************"), msg2.GetContent())
}
