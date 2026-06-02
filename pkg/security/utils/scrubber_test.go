// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubber(t *testing.T) {
	t.Run("cmdline-default", func(t *testing.T) {
		scrubber, err := NewScrubber(nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"cmd --secret 1234567890 --token 1234567890 --jwt abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--jwt", "********"}, scrubbed)

		scrubbed = scrubber.ScrubCommand([]string{"cmd", "--secret", "1234567890", "--token", "1234567890", "--jwt", "abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--jwt", "********"}, scrubbed)
	})

	t.Run("cmdline-custom-word", func(t *testing.T) {
		scrubber, err := NewScrubber([]string{"custom"}, nil)
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"cmd --secret 1234567890 --token 1234567890 --custom abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--custom", "********"}, scrubbed)

		scrubbed = scrubber.ScrubCommand([]string{"cmd", "--secret", "1234567890", "--token", "1234567890", "--custom", "abc"})
		assert.Equal(t, []string{"cmd", "--secret", "********", "--token", "********", "--custom", "********"}, scrubbed)
	})

	t.Run("cmdline-custom-regexp", func(t *testing.T) {
		scrubber, err := NewScrubber(nil, []string{"a[a-z]*c", "t.st"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"--abc 1234567890 --test 1234567890"})
		assert.Equal(t, []string{"--***** 1234567890 --***** 1234567890"}, scrubbed)
	})

	t.Run("line-custom-regexp", func(t *testing.T) {
		scrubber, err := NewScrubber(nil, []string{"a[a-z]*c", "t.st"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubLine("abc 1234567890 test 1234567890")
		assert.Equal(t, "***** 1234567890 ***** 1234567890", scrubbed)
	})
}
