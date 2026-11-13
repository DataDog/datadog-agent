// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestUsageStream checks when the runtime usage of packages stops being
// awaited, and what brings it back.
func TestUsageStream(t *testing.T) {
	const wait = 2 * time.Minute
	start := time.Unix(1700000000, 0)

	t.Run("usage can come before the stream tells", func(t *testing.T) {
		var s UsageStream
		assert.True(t, s.Available(start, wait))
	})

	t.Run("failed opens stop usage after wait", func(t *testing.T) {
		var s UsageStream
		s.OpenFailed(start)
		s.OpenFailed(start.Add(time.Minute))
		assert.True(t, s.Available(start.Add(wait-time.Second), wait), "the failures count from the first")
		assert.False(t, s.Available(start.Add(wait), wait))

		s.Opened()
		assert.True(t, s.Available(start.Add(wait), wait), "an open stream ends the failures")

		s.OpenFailed(start.Add(wait))
		assert.True(t, s.Available(start.Add(2*wait-time.Second), wait), "new failures count from the first of them")
	})

	t.Run("an answer without the service stops usage", func(t *testing.T) {
		var s UsageStream
		assert.True(t, s.Unserved(), "the first answer is news")
		assert.False(t, s.Unserved(), "the next ones are not")
		assert.False(t, s.Available(start, wait))

		s.Opened()
		assert.False(t, s.Available(start, wait), "an open stream has yet to show the service")
	})

	t.Run("a report brings usage back", func(t *testing.T) {
		var s UsageStream
		s.Unserved()
		s.OpenFailed(start)
		s.Reported()
		assert.True(t, s.Available(start.Add(wait), wait))
		assert.True(t, s.Unserved(), "an answer without the service is news again")
	})
}
