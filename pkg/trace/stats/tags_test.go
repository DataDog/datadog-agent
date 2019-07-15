// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package stats

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroup(t *testing.T) {
	cases := map[string]string{
		"a:1":   "a",
		"a":     "",
		"a:1:1": "a",
		"abc:2": "abc",
	}

	assert := assert.New(t)
	for in, out := range cases {
		actual := TagGroup(in)
		assert.Equal(out, actual)
	}
}

func TestSort(t *testing.T) {
	t1 := NewTagSetFromString("a:2,a:1,a:3")
	t2 := NewTagSetFromString("a:1,a:2,a:3")
	sort.Sort(t1)
	assert.Equal(t, t1, t2)

	// trick: service<name but mcnulty<query (traps a bug if we consider
	// that "if not name1 < name2 then compare value1 and value2")
	t1 = NewTagSetFromString("mymetadata:cool,service:mcnulty,name:query")
	t2 = NewTagSetFromString("mymetadata:cool,name:query,service:mcnulty")
	sort.Sort(t1)
	sort.Sort(t2)
	assert.Equal(t, t1, t2)
}

func TestMergeTagSets(t *testing.T) {
	t1 := NewTagSetFromString("a:1,a:2")
	t2 := NewTagSetFromString("a:2,a:3")
	t3 := MergeTagSets(t1, t2)
	assert.Equal(t, t3, NewTagSetFromString("a:1,a:2,a:3"))

	t1 = NewTagSetFromString("a:1")
	t2 = NewTagSetFromString("a:2")
	t3 = MergeTagSets(t1, t2)
	assert.Equal(t, t3, NewTagSetFromString("a:1,a:2"))

	t1 = NewTagSetFromString("a:2,a:1")
	t2 = NewTagSetFromString("a:6,a:2")
	t3 = MergeTagSets(t1, t2)
	assert.Equal(t, t3, NewTagSetFromString("a:1,a:2,a:6"))

	t1 = nil
	t2 = NewTagSetFromString("a:6,a:2")
	t3 = MergeTagSets(t1, t2)
	assert.Equal(t, t3, t2)

	t1 = NewTagSetFromString("a:2,a:1")
	t2 = nil
	t3 = MergeTagSets(t1, t2)
	assert.Equal(t, t3, t1)
}

func TestSplitTag(t *testing.T) {
	group, value := SplitTag("a:b")
	assert.Equal(t, group, "a")
	assert.Equal(t, value, "b")

	group, value = SplitTag("k:v:w")
	assert.Equal(t, group, "k")
	assert.Equal(t, value, "v:w")

	group, value = SplitTag("a")
	assert.Equal(t, group, "")
	assert.Equal(t, value, "a")
}

func TestTagColon(t *testing.T) {
	ts := NewTagSetFromString("a:1:2:3,url:http://localhost:1234/")
	t.Logf("ts: %v", ts)
	assert.Equal(t, "1:2:3", ts.Get("a").Value)
	assert.Equal(t, "http://localhost:1234/", ts.Get("url").Value)
}

func TestFilterTags(t *testing.T) {
	assert := assert.New(t)

	cases := []struct {
		tags, groups, out []string
	}{
		{
			tags:   []string{"a:1", "a:2", "b:1", "c:2"},
			groups: []string{"a", "b"},
			out:    []string{"a:1", "a:2", "b:1"},
		},
		{
			tags:   []string{"a:1", "a:2", "b:1", "c:2"},
			groups: []string{"b"},
			out:    []string{"b:1"},
		},
		{
			tags:   []string{"a:1", "a:2", "b:1", "c:2"},
			groups: []string{"d"},
			out:    nil,
		},
		{
			tags:   nil,
			groups: []string{"d"},
			out:    nil,
		},
	}

	for _, c := range cases {
		out := FilterTags(c.tags, c.groups)
		assert.Equal(out, c.out)
	}

}
func TestTagSetUnset(t *testing.T) {
	assert := assert.New(t)

	ts1 := NewTagSetFromString("service:mcnulty,resource:template,custom:mymetadata")
	assert.Len(ts1, 3)
	assert.Equal("mcnulty", ts1.Get("service").Value)
	assert.Equal("template", ts1.Get("resource").Value)
	assert.Equal("mymetadata", ts1.Get("custom").Value)

	ts2 := ts1.Unset("resource") // remove at the middle
	assert.Len(ts2, 2)
	assert.Equal("mcnulty", ts2.Get("service").Value)
	assert.Equal("", ts2.Get("resource").Value)
	assert.Equal("mymetadata", ts1.Get("custom").Value)

	ts3 := ts2.Unset("custom") // remove at the end
	assert.Len(ts3, 1)
	assert.Equal("mcnulty", ts3.Get("service").Value)
	assert.Equal("", ts3.Get("resource").Value)
	assert.Equal("", ts3.Get("name").Value)
}

func TestTagSetKey(t *testing.T) {
	ts := NewTagSetFromString("a:b,a:b:c,abc")
	assert.Equal(t, ":abc,a:b,a:b:c", ts.Key())
}
