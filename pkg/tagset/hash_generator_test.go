// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagsOrderAndDupsDontMatter(t *testing.T) {
	assert := assert.New(t)

	tags := []string{"bar", "foo", "key:value", "key:value2"}

	hg := NewHashGenerator()
	tagsBuf := NewHashingTagsAccumulatorWithTags(tags)
	key := hg.Hash(tagsBuf)

	// change tags order, the generated key should be the same
	tags[0], tags[1], tags[2], tags[3] = tags[3], tags[0], tags[1], tags[2]
	tagsBuf2 := NewHashingTagsAccumulatorWithTags(tags)
	key2 := hg.Hash(tagsBuf2)
	assert.Equal(key, key2, "order of tags should not matter")

	// add a duplicated tag
	tags = append(tags, "key:value", "foo")
	tagsBuf3 := NewHashingTagsAccumulatorWithTags(tags)
	key3 := hg.Hash(tagsBuf3)
	assert.Equal(key, key3, "duplicated tags should not matter")
	assert.Equal(tagsBuf2.Get(), tagsBuf3.Get(), "duplicated tags should be removed from the buffer")

	// and now, completely change of the tag, the generated key should NOT be the same
	tags[2] = "another:tag"
	key4 := hg.Hash(NewHashingTagsAccumulatorWithTags(tags))
	assert.NotEqual(key, key4, "tags content should matter")
}

func TestTagsAreDedupedWhileGeneratingCKey(t *testing.T) {
	withSizeAndSeed := func(size, iterations int, seed int64) func(*testing.T) {
		return func(t *testing.T) {
			assert := assert.New(t)
			r := rand.New(rand.NewSource(seed))
			tags, expUniq := genTags(size, 2)
			tagsBuf := NewHashingTagsAccumulatorWithTags(tags)

			hg := NewHashGenerator()
			expKey := hg.Hash(tagsBuf.Dup())
			for i := 0; i < iterations; i++ {
				tags := tagsBuf.Copy()
				r.Shuffle(size, func(i, j int) { tags[i], tags[j] = tags[j], tags[i] })
				tagsBuf := NewHashingTagsAccumulatorWithTags(tags)
				key := hg.Hash(tagsBuf)
				assert.Equal(expKey, key, "order of tags should not matter")

				newTags := tagsBuf.Get()
				newUniq := make(map[string]int, len(newTags))
				// make sure every tag occurs only once
				for _, tag := range newTags {
					newUniq[tag]++
					assert.Equal(newUniq[tag], 1)
				}
				// make sure all unique tags are present
				for _, tag := range expUniq {
					assert.Equal(newUniq[tag], 1)
				}
			}
		}
	}
	t.Run("smallish", withSizeAndSeed(3, 200, 0x398192f0a9c0))
	t.Run("bigger", withSizeAndSeed(50, 100, 0x398192f0a9c0))
	t.Run("huge", withSizeAndSeed(600, 10, 0x398192f0a9c0))
}

// global variable to avoid undesirable optimization in benchmarks
var Hash uint64

func BenchmarkHashGeneration(b *testing.B) {
	for i := 1; i < 4096; i *= 2 {
		tags, _ := genTags(i, 1)
		tagsBuf := NewHashingTagsAccumulatorWithTags(tags)
		b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
			hg := NewHashGenerator()
			tags := tagsBuf.Dup()
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				Hash = hg.Hash(tags)
			}
		})

	}
}

func genTags(count int, div int) ([]string, []string) {
	var tags []string
	uniqMap := make(map[string]struct{})
	for i := 0; i < count; i++ {
		tag := fmt.Sprintf("tag%d:value%d", i/div, i/div)
		tags = append(tags, tag)
		uniqMap[tag] = struct{}{}
	}

	uniq := []string{}
	for tag := range uniqMap {
		uniq = append(uniq, tag)
	}

	return tags, uniq
}

func TestDedup2Small(t *testing.T) {
	g := NewHashGenerator()

	cases := []struct {
		name               string
		l, r, expL, expR   string
		expHashL, expHashR uint64
	}{
		{"empty", "", "", "", "", 0, 0},
		{"empty l small", "", "foo", "", "foo", 0, 0xe271865701f54561},
		{"empty r small", "foo", "", "foo", "", 0xe271865701f54561, 0},
		// bruteforce mode (# tags <= 4)
		{"small-1", "foo,foo,bar", "ook", "foo,bar", "ook", 0x7047de8cfccfa365, 0xaf143bb3d715f7c5},
		{"small-2", "foo,bar", "foo,ook", "foo,bar", "ook", 0x7047de8cfccfa365, 0xaf143bb3d715f7c5},
		{"small-3", "foo", "ook,foo,eek", "foo", "ook,eek", 0xe271865701f54561, 0x5cc6d96d47bebee3},
		{"small-4", "foo,bar,foo", "", "foo,bar", "", 0x7047de8cfccfa365, 0},
		{"small-5", "", "bar,foo,bar,bar", "", "bar,foo", 0, 0x7047de8cfccfa365},
		// hashing mode (# tags > 4)
		{"mid-1", "foo,bar,bar", "bar,bar,bar,foo", "foo,bar", "", 0x7047de8cfccfa365, 0},
		{"mid-2", "foo,foo,bar", "ook,ook", "foo,bar", "ook", 0x7047de8cfccfa365, 0xaf143bb3d715f7c5},
		{"mid-3", "foo,foo,bar", "foo,ook,bar,eek", "foo,bar", "eek,ook", 0x7047de8cfccfa365, 0x5cc6d96d47bebee3},
		{"mid-4", "bar,bar,foo", "foo,ook,bar,eek,ook", "bar,foo", "ook,eek", 0x7047de8cfccfa365, 0x5cc6d96d47bebee3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewHashingTagsAccumulatorWithTags(strings.Split(tc.l, ","))
			r := NewHashingTagsAccumulatorWithTags(strings.Split(tc.r, ","))

			b := NewHashingTagsAccumulator()
			b.Append(l.Get()...)
			b.Append(r.Get()...)

			g.Dedup2(l, r)
			got := l.Hash() ^ r.Hash()
			exp := g.Hash(b)

			assert.EqualValues(t, exp, got)
			assert.EqualValues(t, tc.expL, strings.Join(l.Get(), ","))
			assert.EqualValues(t, tc.expR, strings.Join(r.Get(), ","))
			assert.EqualValues(t, tc.expHashL, l.Hash())
			assert.EqualValues(t, tc.expHashR, r.Hash())
		})
	}
}

func TestDedup2Rand(t *testing.T) {
	g := NewHashGenerator()

	for i := 1; i <= 1024; i *= 4 {
		for j := 1; j < 4; j++ {
			for k := 0; k < 20; k++ {
				tags, _ := genTags(i, j)
				rand.Shuffle(len(tags), func(i, j int) { tags[i], tags[j] = tags[j], tags[i] })
				for n := 0; n <= len(tags); n += len(tags)/5 + 1 {
					b := NewHashingTagsAccumulatorWithTags(tags)
					l := NewHashingTagsAccumulatorWithTags(tags[:n])
					r := NewHashingTagsAccumulatorWithTags(tags[n:])

					h1 := g.Hash(b)
					g.Dedup2(l, r)
					h2 := l.Hash() ^ r.Hash()

					assert.EqualValues(t, h1, h2)
					l.data = append(l.data, r.data...)
					l.hash = append(l.hash, r.hash...)

					sort.Sort(b)
					sort.Sort(l)

					assert.EqualValues(t, b.Get(), l.Get())
				}
			}
		}
	}
}

func BenchmarkDedup2(b *testing.B) {
	tags, _ := genTags(2048, 1)
	for i := 1; i < 4096; i *= 2 {
		l := NewHashingTagsAccumulatorWithTags(tags[:i/2])
		r := NewHashingTagsAccumulatorWithTags(tags[i/2 : i])
		b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
			hg := NewHashGenerator()
			l, r := l.Dup(), r.Dup()
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				hg.Dedup2(l, r)
			}
		})
	}
}
