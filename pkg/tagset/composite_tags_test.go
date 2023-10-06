// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tagset

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompositeTagsConstructors(t *testing.T) {
	tags1 := NewCompositeTags([]string{"tag1"}, []string{"tag2"})
	tags2 := CompositeTagsFromSlice([]string{"tag1", "tag2"})
	tags3 := CombineCompositeTagsAndSlice(CompositeTagsFromSlice([]string{"tag1"}), []string{"tag2"})

	require.Equal(t, tags1.Join(","), tags2.Join(","))
	require.Equal(t, tags2.Join(","), tags3.Join(","))
}

func TestCompositeTagsCombineCompositeTagsAndSlice(t *testing.T) {
	// Make sure there is no reallocation later
	tags := make([]string, 1, 10)
	tags[0] = "tag1"
	tags2 := NewCompositeTags(tags, []string{"tag2"})
	tags3 := CombineCompositeTagsAndSlice(tags2, []string{"tag3"})
	tags4 := CombineCompositeTagsAndSlice(tags2, []string{"tag4"})

	assert.Equal(t, "tag1,tag2,tag3", tags3.Join(","))
	assert.Equal(t, "tag1,tag2,tag4", tags4.Join(","))
}

func TestCompositeTagsForEach(t *testing.T) {
	compositeTags := NewCompositeTags([]string{"tag1"}, []string{"tag2"})
	var tags []string
	compositeTags.ForEach(func(tag string) {
		tags = append(tags, tag)
	})
	expectedTags := []string{"tag1", "tag2"}
	r := require.New(t)
	r.EqualValues(expectedTags, tags)

	tags = nil
	r.NoError(compositeTags.ForEachErr(func(tag string) error {
		tags = append(tags, tag)
		return nil
	}))
	require.EqualValues(t, expectedTags, tags)

	r.Error(compositeTags.ForEachErr(func(tag string) error {
		return errors.New("error")
	}))
}

func TestCompositeTagsFind(t *testing.T) {
	compositeTags := NewCompositeTags([]string{"tag1"}, []string{"tag2"})
	r := require.New(t)
	r.True(compositeTags.Find(func(tag string) bool { return tag == "tag1" }))
	r.True(compositeTags.Find(func(tag string) bool { return tag == "tag2" }))
	r.False(compositeTags.Find(func(tag string) bool { return tag == "tag3" }))
}

func TestCompositeTagsLen(t *testing.T) {
	r := require.New(t)
	r.Equal(2, NewCompositeTags([]string{"tag1"}, []string{"tag2"}).Len())
	r.Equal(1, NewCompositeTags([]string{"tag1"}, []string{}).Len())
	r.Equal(1, NewCompositeTags([]string{}, []string{"tag1"}).Len())
}

func TestCompositeTagsJoin(t *testing.T) {
	tags := NewCompositeTags([]string{"tag1"}, []string{"tag2"})
	require.Equal(t, "tag1, tag2", tags.Join(", "))

	tags = CompositeTagsFromSlice([]string{"tag1", "tag2"})
	require.Equal(t, "tag1, tag2", tags.Join(", "))

	tags = NewCompositeTags(nil, []string{"tag1", "tag2"})
	require.Equal(t, "tag1, tag2", tags.Join(", "))
}

func TestCompositeTagsMarshalJSON(t *testing.T) {
	r := require.New(t)
	tags := NewCompositeTags([]string{"tag1"}, []string{"tag2"})

	bytes, err := json.Marshal(tags)
	r.NoError(err)
	var newTags *CompositeTags
	r.NoError(json.Unmarshal(bytes, &newTags))
	require.Equal(t, tags.Join(", "), newTags.Join(", "))
}
