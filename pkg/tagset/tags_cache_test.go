package tagset

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagsCacheCaching(t *testing.T) {
	f := NewNullFactory()
	tc, _ := newTagsCache(100, 1)
	missCalls := 0

	miss := func() *Tags {
		missCalls++
		return f.NewTags([]string{fmt.Sprintf("tag%d", missCalls)})
	}
	t.Run("fresh cache", func(t *testing.T) {
		require.Equal(t, "tag1", tc.getCachedTags(0x12345, miss).String())
		require.Equal(t, 1, missCalls)
	})
	t.Run("cached value", func(t *testing.T) {
		require.Equal(t, "tag1", tc.getCachedTags(0x12345, miss).String())
		require.Equal(t, 1, missCalls)
	})
	t.Run("new cache key", func(t *testing.T) {
		require.Equal(t, "tag2", tc.getCachedTags(0xabcde, miss).String())
		require.Equal(t, 2, missCalls)
	})
	t.Run("old cached value", func(t *testing.T) {
		require.Equal(t, "tag1", tc.getCachedTags(0x12345, miss).String())
		require.Equal(t, 2, missCalls)
	})
}

func TestTagsCacheCachingErr(t *testing.T) {
	f := NewNullFactory()
	tc, _ := newTagsCache(100, 1)
	missCalls := 0
	missErrs := 0

	miss := func() (*Tags, error) {
		missCalls++
		return f.NewTags([]string{fmt.Sprintf("tag%d", missCalls)}), nil
	}

	missErr := func() (*Tags, error) {
		missErrs++
		return nil, errors.New("uhoh")
	}

	t.Run("fresh cache, error", func(t *testing.T) {
		v, err := tc.getCachedTagsErr(0x12345, missErr)
		require.Error(t, err)
		require.Nil(t, v)
		require.Equal(t, 1, missErrs)
	})
	t.Run("same key, no error", func(t *testing.T) {
		v, err := tc.getCachedTagsErr(0x12345, miss)
		require.NoError(t, err)
		require.Equal(t, "tag1", v.String())
		require.Equal(t, 1, missCalls)
		require.Equal(t, 1, missErrs)
	})
	t.Run("cached value", func(t *testing.T) {
		v, err := tc.getCachedTagsErr(0x12345, miss)
		require.NoError(t, err)
		require.Equal(t, "tag1", v.String())
		require.Equal(t, 1, missCalls)
	})
	t.Run("new cache key", func(t *testing.T) {
		v, err := tc.getCachedTagsErr(0xabcde, miss)
		require.NoError(t, err)
		require.Equal(t, "tag2", v.String())
		require.Equal(t, 2, missCalls)
	})
	t.Run("old cached value", func(t *testing.T) {
		v, err := tc.getCachedTagsErr(0x12345, miss)
		require.NoError(t, err)
		require.Equal(t, "tag1", v.String())
		require.Equal(t, 2, missCalls)
	})
	t.Run("old cached value, error", func(t *testing.T) {
		v, err := tc.getCachedTagsErr(0x12345, missErr)
		require.NoError(t, err)
		require.Equal(t, "tag1", v.String())
		require.Equal(t, 2, missCalls)
		require.Equal(t, 1, missErrs)
	})
}

func TestTagsCacheBasicRotation(t *testing.T) {
	f := NewNullFactory()
	checkCached := func(insertsPerRotation, cacheCount, numTagsets int, shouldBe bool) func(*testing.T) {
		return func(t *testing.T) {
			t.Run("getCachedTags", func(t *testing.T) {
				tc, _ := newTagsCache(insertsPerRotation, cacheCount)

				v := tc.getCachedTags(0, func() *Tags { return f.NewTags([]string{"expected"}) })
				require.Equal(t, "expected", v.String())

				for i := 1; i < numTagsets; i++ {
					tc.getCachedTags(uint64(i), func() *Tags { return EmptyTags })
				}

				v = tc.getCachedTags(0, func() *Tags { return f.NewTags([]string{"miss"}) })
				if shouldBe {
					require.Equal(t, "expected", v.String())
				} else {
					require.Equal(t, "miss", v.String())
				}
			})

			t.Run("getCachedTagsErr", func(t *testing.T) {
				tc, _ := newTagsCache(insertsPerRotation, cacheCount)

				v, err := tc.getCachedTagsErr(0, func() (*Tags, error) { return f.NewTags([]string{"expected"}), nil })
				require.NoError(t, err)
				require.Equal(t, "expected", v.String())

				for i := 1; i < numTagsets; i++ {
					tc.getCachedTags(uint64(i), func() *Tags { return EmptyTags })
				}

				v, err = tc.getCachedTagsErr(0, func() (*Tags, error) { return f.NewTags([]string{"miss"}), nil })
				require.NoError(t, err)
				if shouldBe {
					require.Equal(t, "expected", v.String())
				} else {
					require.Equal(t, "miss", v.String())
				}
			})
		}
	}

	t.Run("10 inserts,PerRotation 1 cacheCount, #9 still exists", checkCached(10, 1, 9, true))
	t.Run("10 insertsPerRotation, 1 cacheCount, #10 is expired", checkCached(10, 1, 10, false))
	t.Run("5 inserts,PerRotation 3 cacheCount, #14 still exists", checkCached(5, 3, 14, true))
	t.Run("5 insertsPerRotation, 3 cacheCount, #15 is expired", checkCached(5, 3, 15, false))
}

func TestTagsCacheRecaching(t *testing.T) {
	f := NewNullFactory()
	tc, _ := newTagsCache(10, 3)

	v := tc.getCachedTags(0x9999, func() *Tags { return f.NewTags([]string{"expected"}) })
	require.Equal(t, "expected", v.String())

	// now loop a few times, inserting 15 things (which causes a rotation) and
	// then querying 0x9999 again, which should re-cache it in the new map. So
	// the 0x9999 query should never miss. The re-cache counts as an insert,
	// so we use cacheCount=3, allowing that sometimes 0x9999 slips to the third
	// map.
	for i := 0; i < 20; i++ {
		for j := 0; j < 15; j++ {
			tc.getCachedTags(uint64(i*10+j), func() *Tags { return EmptyTags })
		}

		v := tc.getCachedTags(0x9999, func() *Tags { return f.NewTags([]string{"miss"}) })
		require.Equal(t, "expected", v.String())
	}
}

func TestTagsCacheMinimal(t *testing.T) {
	f := NewNullFactory()

	// use the smallest allowed cache, in case this causes any infinite
	// rotation loops or other bugs. Note that this configuration caches nothing!
	tc, _ := newTagsCache(1, 1)

	v := tc.getCachedTags(0x9999, func() *Tags { return f.NewTags([]string{"expected"}) })
	require.Equal(t, "expected", v.String())

	v = tc.getCachedTags(0x9999, func() *Tags { return f.NewTags([]string{"miss"}) })
	require.Equal(t, "miss", v.String())
}
