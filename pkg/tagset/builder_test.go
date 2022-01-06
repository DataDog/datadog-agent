// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func testBuilder() *Builder {
	bldr := newBuilder(NewNullFactory(), 10)
	return bldr
}

func TestBuilder_Add_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.Add("abc")
	bldr.Add("def")
	bldr.Add("abc")

	tags := bldr.Close()
	tags.validate(t)

	require.Equal(t, tags.Sorted(), []string{"abc", "def"})
}

func TestBuilder_AddKV_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.AddKV("host", "foo")
	bldr.AddKV("host", "bar")
	bldr.AddKV("host", "foo")

	tags := bldr.Close()
	tags.validate(t)

	require.Equal(t, tags.Sorted(), []string{"host:bar", "host:foo"})
}

func TestBuilder_AddTags_Close(t *testing.T) {
	bldr := testBuilder()

	bldr.Add("host:foo")
	bldr.AddTags(NewTags([]string{"abc", "def"}))

	tags := bldr.Close()
	tags.validate(t)

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

	bldr := DefaultFactory.NewBuilder(5)
	for _, shard := range shards {
		bldr.AddKV("shard", strconv.Itoa(shard))
	}

	tags := bldr.Close()

	fmt.Printf("%s", tags.Sorted())
	// Output: [shard:1 shard:19 shard:4]
}
