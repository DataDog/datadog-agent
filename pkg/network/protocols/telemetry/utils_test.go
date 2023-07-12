// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitTagsAndOpts(t *testing.T) {
	assert := assert.New(t)

	t.Run("only tags", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"tag:a", "tag:c", "tag:b"})
		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, tags.List())
		assert.Len(opts, 0)
	})

	t.Run("only opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"_opt3", "_opt2", "_opt1"})
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, opts.List())
		assert.Len(tags, 0)
	})

	t.Run("tags and opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions(
			[]string{"_opt3", "tag:a", "_opt2", "tag:b", "_opt1", "tag:c"},
		)

		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, tags.List())
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, opts.List())
	})

}
