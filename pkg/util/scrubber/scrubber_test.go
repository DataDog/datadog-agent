// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWithDefaults(t *testing.T) {
	scrubber := NewWithDefaults()
	scrubberEmpty := New()
	AddDefaultReplacers(scrubberEmpty)

	assert.NotZero(t, len(scrubber.singleLineReplacers))
	assert.NotZero(t, len(scrubber.multiLineReplacers))
	assert.Equal(t, len(scrubberEmpty.singleLineReplacers), len(scrubber.singleLineReplacers))
	assert.Equal(t, len(scrubberEmpty.multiLineReplacers), len(scrubber.multiLineReplacers))
}

func TestLastUpdated(t *testing.T) {
	scrubber := NewWithDefaults()
	for _, replacer := range scrubber.singleLineReplacers {
		assert.NotNil(t, replacer.LastUpdated, "single line replacer has no LastUpdated: %v", replacer)
	}
	for _, replacer := range scrubber.multiLineReplacers {
		assert.NotNil(t, replacer.LastUpdated, "multi line replacer has no LastUpdated: %v", replacer)
	}
}

func TestRepl(t *testing.T) {
	scrubber := New()
	scrubber.AddReplacer(SingleLine, Replacer{
		Regex: regexp.MustCompile("foo"),
		Repl:  []byte("bar"),
	})
	res, err := scrubber.ScrubBytes([]byte("dog food"))
	require.NoError(t, err)
	require.Equal(t, "dog bard", string(res))
}

func TestReplFunc(t *testing.T) {
	scrubber := New()
	scrubber.AddReplacer(SingleLine, Replacer{
		Regex: regexp.MustCompile("foo"),
		ReplFunc: func(match []byte) []byte {
			return []byte(strings.ToUpper(string(match)))
		},
	})
	res, err := scrubber.ScrubBytes([]byte("dog food"))
	require.NoError(t, err)
	require.Equal(t, "dog FOOd", string(res))
}

func TestSkipComments(t *testing.T) {
	scrubber := New()
	scrubber.AddReplacer(SingleLine, Replacer{
		Regex: regexp.MustCompile("foo"),
		Repl:  []byte("bar"),
	})
	scrubber.AddReplacer(MultiLine, Replacer{
		Regex: regexp.MustCompile("with bar\n\n\nanother"),
		Repl:  []byte("..."),
	})
	res, err := scrubber.ScrubBytes([]byte("a line with foo\n\n  \n  # a comment with foo\nanother line"))
	require.NoError(t, err)
	require.Equal(t, "a line ... line", string(res))
}

func TestCleanFile(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "test.yml")
	os.WriteFile(filename, []byte("a line with foo\n\na line with bar"), 0666)

	scrubber := New()
	scrubber.AddReplacer(SingleLine, Replacer{
		Regex: regexp.MustCompile("foo"),
		Repl:  []byte("bar"),
	})
	res, err := scrubber.ScrubFile(filename)
	require.NoError(t, err)
	require.Equal(t, "a line with bar\n\na line with bar", string(res))
}

func TestScrubLine(t *testing.T) {
	scrubber := New()
	scrubber.AddReplacer(SingleLine, Replacer{
		Regex: regexp.MustCompile(`([A-Za-z][A-Za-z0-9+-.]+\:\/\/|\b)([^\:]+)\:([^\s]+)\@`),
		Repl:  []byte(`$1$2:********@`),
	})
	// this replacer should not be used on URLs!
	scrubber.AddReplacer(MultiLine, Replacer{
		Regex: regexp.MustCompile(".*"),
		Repl:  []byte("UHOH"),
	})
	res := scrubber.ScrubLine("https://foo:bar@example.com")
	require.Equal(t, "https://foo:********@example.com", res)
}

func TestScrubBig(t *testing.T) {
	scrubber := New()
	content := bytes.Repeat([]byte("a"), 1000000)

	_, err := scrubber.ScrubBytes(content)
	require.NoError(t, err)
}

func TestENCTextScrubbing(t *testing.T) {
	scrubber := NewWithDefaults()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"api_key with ENC", "api_key=ENC[secret]", "api_key=ENC[secret]"},
		{"api_key regular", "api_key=aaaaaaaaaaaaaaaaaaaaaaaaaabbbbb", "api_key=***************************bbbbb"},
		{"password with ENC", "password=ENC[secret]", "password=ENC[secret]"},
		{"password regular", "password=apassword", "password=********"},

		{"token YAML ENC", "auth_token: ENC[yaml_token]", "auth_token: ENC[yaml_token]"},
		{"token regular", "auth_token: plain_token_value", "auth_token: \"********\""},

		{"empty ENC", "api_key: ENC[]", "api_key: ENC[]"},
		{"whitespace ENC", "password:   ENC[key]  ", "password:   ENC[key]  "},
		{"invalid ENC", "password: ENC[incomplete", "password: \"********\""},
		{"multiple mixed", "api_key: ENC[valid] password: plain_secret", "api_key: ENC[valid] password: \"********\""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := scrubber.ScrubBytes([]byte(tc.input))
			require.NoError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestENCLineScrubbing(t *testing.T) {
	scrubber := NewWithDefaults()

	testLines := []struct {
		input    string
		expected string
	}{
		{"api_key: ENC[line_secret]", "api_key: ENC[line_secret]"},
		{"password: ENC[line_pass]", "password: ENC[line_pass]"},
		{"token: ENC[line_token]", "token: ENC[line_token]"},
		{"token: plain_secret", "token: \"********\""},
		{"password: plain_pass", "password: \"********\""},
		{"https://user:ENC[url_pass]@host", "https://user:ENC[url_pass]@host"},
		{"https://user:secret123@host", "https://user:********@host"},
	}

	for _, tc := range testLines {
		result := scrubber.ScrubLine(tc.input)
		assert.Equal(t, tc.expected, result, "ScrubLine should preserve ENC[] handlers")
	}
}
