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
