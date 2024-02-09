// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestSplitTagsAndOpts(t *testing.T) {
	assert := assert.New(t)

	t.Run("only tags", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"tag:a", "tag:c", "tag:b"})
		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, sets.List(tags))
		assert.Len(opts, 0)
	})

	t.Run("only opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"_opt3", "_opt2", "_opt1"})
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, sets.List(opts))
		assert.Len(tags, 0)
	})

	t.Run("tags and opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions(
			[]string{"_opt3", "tag:a", "_opt2", "tag:b", "_opt1", "tag:c"},
		)

		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, sets.List(tags))
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, sets.List(opts))
	})

}

func TestSplitName(t *testing.T) {
	Clear()

	t.Run("with namespace", func(t *testing.T) {
		c1 := NewCounter("usm.http.hits")
		namespace, name := splitName(c1)
		assert.Equal(t, "usm.http", namespace)
		assert.Equal(t, "hits", name)
	})

	t.Run("without namespace", func(t *testing.T) {
		c2 := NewCounter("events")
		namespace, name := splitName(c2)
		assert.Equal(t, "", namespace)
		assert.Equal(t, "events", name)
	})
}
