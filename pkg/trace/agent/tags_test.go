package agent

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

func TestNormalizeTag(t *testing.T) {
	for _, tt := range []struct{ in, out string }{
		{in: "ok", out: "ok"},
		{in: "AlsO:ök", out: "also:ök"},
		{in: ":still_ok", out: ":still_ok"},
		{in: "___trim", out: "trim"},
		{in: "12.:trim@", out: ":trim"},
		{in: "12.:trim@@", out: ":trim"},
		{in: "fun:ky__tag/1", out: "fun:ky_tag/1"},
		{in: "fun:ky@tag/2", out: "fun:ky_tag/2"},
		{in: "fun:ky@@@tag/3", out: "fun:ky_tag/3"},
		{in: "tag:1/2.3", out: "tag:1/2.3"},
		{in: "---fun:k####y_ta@#g/1_@@#", out: "fun:k_y_ta_g/1"},
		{in: "AlsO:œ#@ö))œk", out: "also:œ_ö_œk"},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.out, NormalizeTag(tt.in), tt.in)
		})
	}
}

func benchNormalizeTag(tag string) func(b *testing.B) {
	return func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			NormalizeTag(tag)
		}
	}
}

func BenchmarkNormalizeTag(b *testing.B) {
	b.Run("ok", benchNormalizeTag("good_tag"))
	b.Run("trim", benchNormalizeTag("___trim_left"))
	b.Run("trim-both", benchNormalizeTag("___trim_right@@#!"))
	b.Run("plenty", benchNormalizeTag("fun:ky_ta@#g/1"))
	b.Run("more", benchNormalizeTag("fun:k####y_ta@#g/1_@@#"))
}
