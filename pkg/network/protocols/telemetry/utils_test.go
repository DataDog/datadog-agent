// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitTagsAndOpts(t *testing.T) {
	assert := assert.New(t)

	t.Run("only tags", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"tag:a", "tag:c", "tag:b"})
		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, tags)
		assert.Len(opts, 0)
	})

	t.Run("only opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"_opt3", "_opt2", "_opt1"})
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, opts)
		assert.Len(tags, 0)
	})

	t.Run("tags and opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions(
			[]string{"_opt3", "tag:a", "_opt2", "tag:b", "_opt1", "tag:c"},
		)

		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, tags)
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, opts)
	})

}

func TestInsertNestedTagsFor(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		metrics := make(map[string]interface{})
		err := insertNestedValueFor("http.request_count", 1, metrics)
		require.NoError(t, err)
		err = insertNestedValueFor("dns.errors.nxdomain", 5, metrics)
		require.NoError(t, err)
		err = insertNestedValueFor("http.dropped", 10, metrics)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"http": map[string]interface{}{
				"request_count": int64(1),
				"dropped":       int64(10),
			},
			"dns": map[string]interface{}{
				"errors": map[string]interface{}{
					"nxdomain": int64(5),
				},
			},
		}

		assert.Equal(t, expected, metrics)
	})

	t.Run("invalid type", func(t *testing.T) {
		invalidMap := map[string]interface{}{
			"http": "this should another map",
		}
		err := insertNestedValueFor("http.request_count", 1, invalidMap)
		assert.Error(t, err)
	})
}
