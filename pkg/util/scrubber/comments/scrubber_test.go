// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package comments

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScrubLineDoesNothing(t *testing.T) {
	c := NewScrubber()
	require.Equal(t, "# foo", c.ScrubLine("# foo"))
}

func TestScrubComments(t *testing.T) {
	c := NewScrubber()
	scrubbed, err := c.ScrubBytes([]byte(`
# foo
  not scrubbed
  # spaces
	# tabs`))
	require.NoError(t, err)
	require.Equal(t, []byte(`
  not scrubbed
`), scrubbed)
}

func TestNoTrailingNewline(t *testing.T) {
	c := NewScrubber()
	scrubbed, err := c.ScrubBytes([]byte("# comment\nno trailing newline"))
	require.NoError(t, err)
	require.Equal(t, []byte("no trailing newline\n"), scrubbed)
}

func TestTrailingNewline(t *testing.T) {
	c := NewScrubber()
	scrubbed, err := c.ScrubBytes([]byte("# comment\ntrailing newline\n"))
	require.NoError(t, err)
	require.Equal(t, []byte("trailing newline\n"), scrubbed)
}

func TestNoTrailingNewlineComment(t *testing.T) {
	c := NewScrubber()
	scrubbed, err := c.ScrubBytes([]byte("line\n# comment with no newline"))
	require.NoError(t, err)
	require.Equal(t, []byte("line\n"), scrubbed)
}

func TestTrailingNewlineComment(t *testing.T) {
	c := NewScrubber()
	scrubbed, err := c.ScrubBytes([]byte("line\n# comment\n"))
	require.NoError(t, err)
	require.Equal(t, []byte("line\n"), scrubbed)
}
