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
	t.Run("cmdline", func(t *testing.T) {
		scrubber, err := NewScrubber([]string{"token"}, []string{"t[a-z]*n", "t.st"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"--token 1234567890 --test 1234567890"})
		assert.Equal(t, []string{"--***** 1234567890 --***** 1234567890"}, scrubbed)
	})

	t.Run("line", func(t *testing.T) {
		scrubber, err := NewScrubber([]string{"token"}, []string{"t[a-z]*n", "t.st"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubLine("token 1234567890 test 1234567890")
		assert.Equal(t, "***** 1234567890 ***** 1234567890", scrubbed)
	})
}
