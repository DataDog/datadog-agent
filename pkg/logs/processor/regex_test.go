// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestRe2MatchContent(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		content string
		want    bool
	}{
		{"literal match", "hello", "say hello world", true},
		{"literal miss", "hello", "goodbye world", false},
		{"regex match", "h[aeiou]llo", "say hello world", true},
		{"regex miss", "h[aeiou]llo", "say hxllo world", false},
		{"anchored match", "^hello", "hello world", true},
		{"anchored miss", "^hello", "say hello", false},
		{"empty content no match", "hello", "", false},
		{"dot-star matches anything", ".*", "anything", true},
		{"alternation match", "foo|bar", "baz bar qux", true},
		{"alternation miss", "foo|bar", "baz qux", false},
		{"case insensitive match", "(?i)hello", "HELLO", true},
		{"case insensitive miss", "(?i)hello", "HXLLO", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := newProcessingRule(config.ExcludeAtMatch, "", tt.pattern)
			got := re2MatchContent(rule, []byte(tt.content))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRe2MaskReplace(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		placeholder string
		content     string
		wantResult  string
		wantMatched bool
	}{
		{
			name:        "simple literal replacement",
			pattern:     "world",
			placeholder: "[REDACTED]",
			content:     "hello world!",
			wantResult:  "hello [REDACTED]!",
			wantMatched: true,
		},
		{
			name:        "no match",
			pattern:     "world",
			placeholder: "[REDACTED]",
			content:     "hello there",
			wantResult:  "hello there",
			wantMatched: false,
		},
		{
			name:        "multiple replacements",
			pattern:     "secret",
			placeholder: "***",
			content:     "secret1 and secret2",
			wantResult:  "***1 and ***2",
			wantMatched: true,
		},
		{
			name:        "regex pattern replacement",
			pattern:     "api_key=[a-f0-9]+",
			placeholder: "api_key=[REDACTED]",
			content:     "call with api_key=abc123def",
			wantResult:  "call with api_key=[REDACTED]",
			wantMatched: true,
		},
		{
			name:        "backreference replacement",
			pattern:     "([Dd]ata_?values=)\\S+",
			placeholder: "${1}[masked_value]",
			content:     "New data added to Datavalues=123456 on prod",
			wantResult:  "New data added to Datavalues=[masked_value] on prod",
			wantMatched: true,
		},
		{
			name:        "empty content",
			pattern:     "anything",
			placeholder: "x",
			content:     "",
			wantResult:  "",
			wantMatched: false,
		},
		{
			name:        "empty placeholder",
			pattern:     "remove_me",
			placeholder: "",
			content:     "keep remove_me this",
			wantResult:  "keep  this",
			wantMatched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := newProcessingRule(config.MaskSequences, tt.placeholder, tt.pattern)
			gotResult, gotMatched := re2MaskReplace(rule, []byte(tt.content))
			assert.Equal(t, tt.wantMatched, gotMatched)
			assert.Equal(t, tt.wantResult, string(gotResult))
		})
	}
}

func TestRe2MatchContentPreservesLiteralFastPath(t *testing.T) {
	rule := newProcessingRule(config.ExcludeAtMatch, "", "DEBUG")
	assert.True(t, rule.HasLiteralContents(), "pure literal pattern should have literal contents")

	assert.True(t, re2MatchContent(rule, []byte("this is a DEBUG message")))
	assert.False(t, re2MatchContent(rule, []byte("this is an INFO message")))
}

func TestRe2MatchContentAlternationLiteralFastPath(t *testing.T) {
	rule := newProcessingRule(config.ExcludeAtMatch, "", "DEBUG|TRACE")
	assert.True(t, rule.HasLiteralContents(), "alternation of literals should have literal contents")

	assert.True(t, re2MatchContent(rule, []byte("TRACE log line")))
	assert.True(t, re2MatchContent(rule, []byte("DEBUG log line")))
	assert.False(t, re2MatchContent(rule, []byte("INFO log line")))
}

func TestRe2MatchContentRegexFallback(t *testing.T) {
	rule := newProcessingRule(config.ExcludeAtMatch, "", "kube-probe/.*")
	assert.False(t, rule.HasLiteralContents(), "regex pattern should not have literal contents")

	assert.True(t, re2MatchContent(rule, []byte("kube-probe/healthz")))
	assert.False(t, re2MatchContent(rule, []byte("GET /api/v1")))
}

// TestRe2NonUTF8Pattern verifies behavior when the regex *pattern* contains
// bytes that are not valid UTF-8. Both Go's stdlib regexp and RE2 reject
// such patterns at compile time — this is not a go-re2-specific issue.
//
// In practice this cannot happen via normal configuration: YAML is defined
// as UTF-8, so raw UTF-16 bytes would break YAML parsing before reaching
// the regex compiler. Users who need to match specific non-ASCII bytes
// should use hex escape sequences (e.g. `\xff`) which are valid pattern
// syntax in both engines.
func TestRe2NonUTF8Pattern(t *testing.T) {
	t.Run("raw_invalid_utf8_rejected_by_both_engines", func(t *testing.T) {
		// UTF-16LE encoding of 'é' is 0xE9 0x00 — invalid UTF-8.
		pattern := "data=\xe9\x00"
		rules := []*config.ProcessingRule{{
			Type:    config.MaskSequences,
			Name:    "test-utf16",
			Pattern: pattern,
		}}
		err := config.CompileProcessingRules(rules)
		assert.Error(t, err, "patterns with raw invalid UTF-8 should be rejected at compile time")
		assert.Contains(t, err.Error(), "invalid UTF-8")
	})

	t.Run("hex_escape_is_unicode_codepoint_not_raw_byte", func(t *testing.T) {
		// \xff in pattern syntax is U+00FF (ÿ), which in UTF-8 is the
		// two-byte sequence 0xC3 0xBF — NOT the raw byte 0xFF.
		// Both engines behave identically here.
		rule := newProcessingRule(config.MaskSequences, "[MASKED]", `data=\xff`)

		// Matches the UTF-8 encoding of U+00FF (ÿ = 0xC3 0xBF)
		utf8Content := []byte("prefix data=\xc3\xbf suffix")
		got, matched := re2MaskReplace(rule, utf8Content)
		assert.True(t, matched, "\\xff in pattern should match UTF-8 encoding of U+00FF")
		assert.Equal(t, "prefix [MASKED] suffix", string(got))

		// Does NOT match the raw byte 0xFF
		rawContent := []byte("prefix data=\xff suffix")
		got, matched = re2MaskReplace(rule, rawContent)
		assert.False(t, matched, "\\xff in pattern should not match raw byte 0xFF")
		assert.Equal(t, rawContent, got)
	})

	t.Run("hex_range_is_unicode_not_raw_bytes", func(t *testing.T) {
		// [\x80-\xff] matches Unicode codepoints U+0080 to U+00FF,
		// which are all two-byte UTF-8 sequences (0xC2 0x80 to 0xC3 0xBF).
		// It does NOT match raw single bytes 0x80-0xFF.
		rule := newProcessingRule(config.ExcludeAtMatch, "", `marker[\x80-\xff]`)

		// U+00A9 (©) = UTF-8 0xC2 0xA9 — within U+0080..U+00FF range
		assert.True(t, re2MatchContent(rule, []byte("log marker\xc2\xa9 end")),
			"should match UTF-8 encoded codepoint in U+0080..U+00FF range")

		// Raw byte 0x80 — not valid UTF-8, not matched
		assert.False(t, re2MatchContent(rule, []byte("log marker\x80 end")),
			"should not match raw byte 0x80")
	})
}

// TestRe2NonUTF8Content verifies that re2MatchContent and re2MaskReplace
// produce the same results as Go's stdlib regexp when the input content
// contains invalid UTF-8 byte sequences. RE2 defaults to UTF-8 mode, so
// we need to ensure it doesn't silently skip matches or produce wrong
// offsets on binary/malformed log lines.
func TestRe2NonUTF8Content(t *testing.T) {
	// Common invalid UTF-8 sequences
	invalidSeqs := [][]byte{
		{0x80},                   // continuation byte without start
		{0xC0, 0xAF},             // overlong encoding of '/'
		{0xED, 0xA0, 0x80},       // surrogate half (U+D800)
		{0xF4, 0x90, 0x80, 0x80}, // above U+10FFFF
		{0xFF, 0xFE},             // never valid
	}

	t.Run("match_surrounding_invalid_bytes", func(t *testing.T) {
		rule := newProcessingRule(config.ExcludeAtMatch, "", "secret")
		for i, inv := range invalidSeqs {
			content := append(append([]byte("pre "), inv...), []byte(" secret post")...)
			got := re2MatchContent(rule, content)
			assert.True(t, got, "invalidSeq[%d]: should find 'secret' even with invalid UTF-8 prefix", i)
		}
	})

	t.Run("match_between_invalid_bytes", func(t *testing.T) {
		rule := newProcessingRule(config.ExcludeAtMatch, "", "api_key=[a-f0-9]+")
		for i, inv := range invalidSeqs {
			content := append(append(inv, []byte("api_key=abc123")...), inv...)
			got := re2MatchContent(rule, content)
			assert.True(t, got, "invalidSeq[%d]: should find api_key pattern surrounded by invalid bytes", i)
		}
	})

	t.Run("mask_replace_with_invalid_bytes", func(t *testing.T) {
		rule := newProcessingRule(config.MaskSequences, "[REDACTED]", "secret=[a-z]+")
		for i, inv := range invalidSeqs {
			// Build: "<invalid>prefix secret=abc<invalid> suffix"
			content := append([]byte{}, inv...)
			content = append(content, []byte("prefix secret=abc")...)
			content = append(content, inv...)
			content = append(content, []byte(" suffix")...)

			expected := append([]byte{}, inv...)
			expected = append(expected, []byte("prefix [REDACTED]")...)
			expected = append(expected, inv...)
			expected = append(expected, []byte(" suffix")...)

			got, matched := re2MaskReplace(rule, content)
			assert.True(t, matched, "invalidSeq[%d]: should match", i)
			assert.Equal(t, expected, got, "invalidSeq[%d]: replacement should preserve invalid bytes and produce correct offsets", i)
		}
	})

	t.Run("mask_replace_multiple_matches_with_invalid_bytes", func(t *testing.T) {
		rule := newProcessingRule(config.MaskSequences, "***", "token")
		// "token<0xFF 0xFE>token"
		content := []byte("token\xFF\xFEtoken")
		got, matched := re2MaskReplace(rule, content)
		assert.True(t, matched)
		assert.Equal(t, []byte("***\xFF\xFE***"), got,
			"both occurrences of 'token' should be replaced, invalid bytes preserved between them")
	})

	t.Run("no_false_positive_on_pure_binary", func(t *testing.T) {
		rule := newProcessingRule(config.ExcludeAtMatch, "", "password")
		content := []byte{0x80, 0x81, 0xFE, 0xFF, 0x00, 0xC0, 0xAF}
		got := re2MatchContent(rule, content)
		assert.False(t, got, "pure binary content should not match ASCII pattern")
	})
}
