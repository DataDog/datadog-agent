// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testFactory(t *testing.T, factoryFactory func() Factory) {
	t.Run("NewTags", func(t *testing.T) {
		f := factoryFactory()
		tags := f.NewTags([]string{"tag1", "tag2", "tag1", "tag2"})
		tags.validate(t)
		require.Equal(t, []string{"tag1", "tag2"}, tags.Sorted())
	})

	t.Run("NewTags_NoDupes", func(t *testing.T) {
		f := factoryFactory()
		tags := f.NewTags([]string{"tag1", "tag2"})
		tags.validate(t)
		require.Equal(t, []string{"tag1", "tag2"}, tags.Sorted())
	})

	t.Run("NewUniqueTags", func(t *testing.T) {
		f := factoryFactory()
		tags := f.NewUniqueTags("tag1", "tag2")
		tags.validate(t)
		require.Equal(t, []string{"tag1", "tag2"}, tags.Sorted())
	})

	t.Run("NewTagsFromMap", func(t *testing.T) {
		f := factoryFactory()
		tags := f.NewTagsFromMap(map[string]struct{}{
			"tag1": {},
			"tag2": {},
		})
		tags.validate(t)
		require.Equal(t, []string{"tag1", "tag2"}, tags.Sorted())
	})

	t.Run("NewTag", func(t *testing.T) {
		f := factoryFactory()
		tags := f.NewTag("foo")
		tags.validate(t)
		require.Equal(t, []string{"foo"}, tags.Sorted())
	})

	t.Run("NewBuilder", func(t *testing.T) {
		f := factoryFactory()
		b := f.NewBuilder(2)
		b.Add("t1")
		b.Add("t2")
		tags := b.Close()
		tags.validate(t)

		require.Equal(t, []string{"t1", "t2"}, tags.Sorted())
	})

	t.Run("Union_Overlapping", func(t *testing.T) {
		f := factoryFactory()
		tags1 := f.NewTags([]string{"tag1", "tag2"})
		tags2 := f.NewTags([]string{"tag2", "tag3"})
		tags := f.Union(tags1, tags2)
		tags.validate(t)

		require.Equal(t, []string{"tag1", "tag2", "tag3"}, tags.Sorted())
	})

	t.Run("Union_NonOverlapping", func(t *testing.T) {
		f := factoryFactory()
		tags1 := f.NewTags([]string{"tag1", "tag2"})
		tags2 := f.NewTags([]string{"tag5", "tag6"})
		tags := f.Union(tags1, tags2)
		tags.validate(t)

		require.Equal(t, []string{"tag1", "tag2", "tag5", "tag6"}, tags.Sorted())
	})
}

func testFactoryCaching(t *testing.T, factoryFactory func() Factory) {
	t.Run("Caching", func(t *testing.T) {
		t.Run("NewTags", func(t *testing.T) {
			f := factoryFactory()
			tags1 := f.NewTags([]string{"tag1", "tag2"})
			tags2 := f.NewTags([]string{"tag2", "tag1"})

			// check for pointer equality
			require.True(t, tags1 == tags2)
		})

		t.Run("NewTagsFromMap", func(t *testing.T) {
			f := factoryFactory()
			tags1 := f.NewTagsFromMap(map[string]struct{}{
				"tag1": {},
				"tag2": {},
			})
			tags2 := f.NewTagsFromMap(map[string]struct{}{
				"tag2": {},
				"tag1": {},
			})

			// check for pointer equality
			require.True(t, tags1 == tags2)
		})

		t.Run("NewTag", func(t *testing.T) {
			f := factoryFactory()
			tag1a := f.NewTag("tag1")
			tag1b := f.NewTag("tag1")
			tag2 := f.NewTag("tag2")

			// check for pointer equality
			require.True(t, tag1a == tag1b)
			require.True(t, tag1a != tag2)
		})

		t.Run("NewBuilder", func(t *testing.T) {
			f := factoryFactory()
			tags1 := f.NewTags([]string{"t1", "t2"})
			tags1.validate(t)

			b := f.NewBuilder(2)
			b.Add("t1")
			b.Add("t2")
			tags2 := b.Close()
			tags2.validate(t)

			// check for pointer equality
			require.True(t, tags1 == tags2)
		})

		t.Run("Union", func(t *testing.T) {
			f := factoryFactory()
			tags12 := f.NewTags([]string{"tag1", "tag2"})
			tags23 := f.NewTags([]string{"tag2", "tag3"})
			tags13 := f.NewTags([]string{"tag1", "tag3"})
			allTagsA := f.Union(tags12, tags23)
			allTagsA.validate(t)
			allTagsB := f.Union(tags13, tags23)
			allTagsB.validate(t)

			// check for pointer equality
			require.True(t, allTagsA == allTagsB)
		})

		t.Run("Union_CacheCollision", func(t *testing.T) {
			f := factoryFactory()

			tags12 := f.NewTags([]string{"tag1", "tag2"})
			tags34 := f.NewTags([]string{"tag3", "tag4"})
			tags1234 := f.Union(tags12, tags34)
			tags1234.validate(t)

			tags12x := f.NewTags([]string{"tag1", "tag2", "xx"})
			tags34x := f.NewTags([]string{"tag3", "tag4", "xx"})
			tags1234x := f.Union(tags12x, tags34x)
			tags1234x.validate(t)

			require.Equal(t, []string{"tag1", "tag2", "tag3", "tag4"}, tags1234.Sorted())
			require.Equal(t, []string{"tag1", "tag2", "tag3", "tag4", "xx"}, tags1234x.Sorted())
		})
	})
}
