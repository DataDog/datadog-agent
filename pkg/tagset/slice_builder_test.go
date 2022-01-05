package tagset

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSliceBuilder_Fuzz(t *testing.T) {
	f := NewNullFactory()
	var lastSb *SliceBuilder
	fuzz(t, func(seed int64) {
		r := rand.New(rand.NewSource(seed))
		levels := r.Intn(10) + 1

		sb := f.NewSliceBuilder(levels, 0)

		// check that we got the same slicebuilder (so it's being reused, behavior
		// we want to validate here)
		if lastSb != nil {
			require.True(t, sb == lastSb, "factory did not reuse SliceBuilder")
		}
		lastSb = sb

		// track the expected values by using an array of builders
		builders := make([]*Builder, levels)
		for l := 0; l < levels; l++ {
			builders[l] = f.NewBuilder(0)
		}

		// add unique tags to the builders
		for tagnum := 0; tagnum < r.Intn(100); tagnum++ {
			level := r.Intn(levels)
			tag := fmt.Sprintf("%d:%d", level, tagnum)
			sb.Add(level, tag)
			builders[level].Add(tag)
		}

		// freeze the builders into tags
		expectedTags := make([]*Tags, levels)
		for l := 0; l < levels; l++ {
			expectedTags[l] = builders[l].Close()
		}

		// verify that each slice is correct (including empty slices)
		for a := 0; a < levels; a++ {
			for b := a; b < levels+1; b++ {
				tags := sb.FreezeSlice(a, b)
				tags.validate(t)

				// union all of the expected tags together
				exp := EmptyTags
				for l := a; l < b; l++ {
					exp = f.Union(exp, expectedTags[l])
				}

				require.Equal(t, exp.Sorted(), tags.Sorted())
			}
		}

		sb.Close()
	})
}

func TestSliceBuilder_AddKV(t *testing.T) {
	f := NewNullFactory()
	sb := f.NewSliceBuilder(3, 4)

	sb.AddKV(0, "host", "123")
	sb.AddKV(0, "cluster", "k")
	sb.AddKV(1, "container", "abc")
	sb.AddKV(2, "task", "92489")

	t0 := sb.FreezeSlice(0, 1)
	t0.validate(t)
	require.Equal(t, []string{"cluster:k", "host:123"}, t0.Sorted())

	t01 := sb.FreezeSlice(0, 2)
	t01.validate(t)
	require.Equal(t, []string{"cluster:k", "container:abc", "host:123"}, t01.Sorted())

	t012 := sb.FreezeSlice(0, 3)
	t012.validate(t)
	require.Equal(t, []string{"cluster:k", "container:abc", "host:123", "task:92489"}, t012.Sorted())
}

func ExampleSliceBuilder() {
	regions := []string{"emea", "us", "antarctic"}
	datasets := []string{"data.world", "kaggle"}
	shards := []int{1, 4, 19}

	bldr := DefaultFactory.NewSliceBuilder(3, 5) // stage 1: adding tags
	for _, rgn := range regions {
		bldr.AddKV(0, "region", rgn)
	}
	for _, ds := range datasets {
		bldr.AddKV(1, "dataset", ds)
	}
	for _, shard := range shards {
		bldr.AddKV(2, "shard", strconv.Itoa(shard))
	}

	lowCardTags := bldr.FreezeSlice(0, 1) // stage 2: frozen
	medCardTags := bldr.FreezeSlice(0, 2)
	allTags := bldr.FreezeSlice(0, 3)

	bldr.Close() // stage 3: closed

	fmt.Printf("%s\n", lowCardTags.Sorted())
	fmt.Printf("%s\n", medCardTags.Sorted())
	fmt.Printf("%s\n", allTags.Sorted())
	// Output:
	// [region:antarctic region:emea region:us]
	// [dataset:data.world dataset:kaggle region:antarctic region:emea region:us]
	// [dataset:data.world dataset:kaggle region:antarctic region:emea region:us shard:1 shard:19 shard:4]
}
