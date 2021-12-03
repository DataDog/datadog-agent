package tagset

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func testBuilder() *Builder {
	bldr := newBuilder(newNullFactory())
	bldr.reset(10)
	return bldr
}

func TestBuilder_Add_Freeze_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.Add("abc")
	bldr.Add("def")
	bldr.Add("abc")

	tags := bldr.Freeze()
	tags.validate(t)
	tags2 := bldr.Freeze()
	tags2.validate(t)
	bldr.Close()

	require.Equal(t, tags, tags2)
	require.Equal(t, tags.Sorted(), []string{"abc", "def"})
}

func TestBuilder_AddKV_Freeze_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.AddKV("host", "foo")
	bldr.AddKV("host", "bar")
	bldr.AddKV("host", "foo")

	tags := bldr.Freeze()
	tags.validate(t)
	bldr.Close()

	require.Equal(t, tags.Sorted(), []string{"host:bar", "host:foo"})
}

func TestBuilder_AddTags_Freeze_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.Add("host:foo")
	bldr.AddTags(NewTags([]string{"abc", "def"}))

	tags := bldr.Freeze()
	tags.validate(t)
	bldr.Close()

	require.Equal(t, tags.Sorted(), []string{"abc", "def", "host:foo"})
}

func TestBuilder_Contains(t *testing.T) {
	bldr := testBuilder()

	bldr.AddKV("host", "foo")
	bldr.AddKV("host", "bar")
	bldr.AddKV("host", "foo")

	require.True(t, bldr.Contains("host:foo"))
	require.False(t, bldr.Contains("host:bing"))
}

func ExampleBuilder() {
	shards := []int{1, 4, 19}

	bldr := DefaultFactory.NewBuilder(5) // stage 1: adding tags
	for _, shard := range shards {
		bldr.AddKV("shard", strconv.Itoa(shard))
	}

	tags := bldr.Freeze() // stage 2: frozen
	bldr.Close()          // stage 3: closed

	fmt.Printf("%s", tags.Sorted())
	// Output: [shard:1 shard:19 shard:4]
}
