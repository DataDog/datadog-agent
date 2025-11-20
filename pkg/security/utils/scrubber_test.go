// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubber(t *testing.T) {
	t.Run("cmdline", func(t *testing.T) {
		scrubber, err := NewScrubber([]string{"token", "password"}, []string{"t[a-z]*n", "p.ssw.rd"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubCommand([]string{"--token 1234567890 --password 1234567890"})
		assert.Equal(t, []string{"--***** 1234567890 --***** 1234567890"}, scrubbed)
	})

	t.Run("line", func(t *testing.T) {
		scrubber, err := NewScrubber([]string{"token", "password"}, []string{"t[a-z]*n", "p.ssw.rd"})
		assert.NoError(t, err)
		assert.NotNil(t, scrubber)

		scrubbed := scrubber.ScrubLine("token 1234567890 password 1234567890")
		assert.Equal(t, "***** 1234567890 ***** 1234567890", scrubbed)
	})
}
