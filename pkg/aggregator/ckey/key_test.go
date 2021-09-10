// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestIsZero(t *testing.T) {
	var k ContextKey
	assert.True(t, k.IsZero())
}

func TestGenerateReproductible(t *testing.T) {
	name := "metric.name"
	hostname := "hostname"
	tags := util.NewHashingTagsBuilderFromSlice([]string{"bar", "foo", "key:value", "key:value2"})

	generator := NewKeyGenerator()

	firstKey := generator.Generate(name, hostname, tags)
	assert.Equal(t, ContextKey(0x558b3fdbeb2ae197), firstKey)

	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key := generator.Generate(name, hostname, tags)
			assert.Equal(t, firstKey, key)
		})
	}

	otherKey := generator.Generate("othername", hostname, tags)
	assert.NotEqual(t, firstKey, otherKey)
	assert.Equal(t, ContextKey(0x76fd4f64609a9375), otherKey)
}

func TestCompare(t *testing.T) {
	base := ContextKey(uint64(0xff3bca32c0520309))
	same := ContextKey(uint64(0xff3bca32c0520309))
	diff := ContextKey(uint64(0xcd3bca32c0520309))

	assert.True(t, Equals(base, base))
	assert.True(t, Equals(base, same))
	assert.False(t, Equals(base, diff))
}

func TestTagsOrderAndDupsDontMatter(t *testing.T) {
	assert := assert.New(t)

	name := "metrics.to.test.hashing"
	hostname := "hostname.localhost"
	tags := []string{"bar", "foo", "key:value", "key:value2"}

	generator := NewKeyGenerator()
	tagsBuf := util.NewHashingTagsBuilderFromSlice(tags)
	key := generator.Generate(name, hostname, tagsBuf)

	// change tags order, the generated key should be the same
	tags[0], tags[1], tags[2], tags[3] = tags[3], tags[0], tags[1], tags[2]
	tagsBuf2 := util.NewHashingTagsBuilderFromSlice(tags)
	key2 := generator.Generate(name, hostname, tagsBuf2)
	assert.Equal(key, key2, "order of tags should not matter")

	// add a duplicated tag
	tags = append(tags, "key:value", "foo")
	tagsBuf3 := util.NewHashingTagsBuilderFromSlice(tags)
	key3 := generator.Generate(name, hostname, tagsBuf3)
	assert.Equal(key, key3, "duplicated tags should not matter")
	assert.Equal(tagsBuf2.Get(), tagsBuf3.Get(), "duplicated tags should be removed from the buffer")

	// and now, completely change of the tag, the generated key should NOT be the same
	tags[2] = "another:tag"
	key4 := generator.Generate(name, hostname, util.NewHashingTagsBuilderFromSlice(tags))
	assert.NotEqual(key, key4, "tags content should matter")
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

func TestTagsAreDedupedWhileGeneratingCKey(t *testing.T) {
	withSizeAndSeed := func(size, iterations int, seed int64) func(*testing.T) {
		return func(t *testing.T) {
			assert := assert.New(t)
			r := rand.New(rand.NewSource(seed))
			name := "metrics.to.test.hashing"
			hostname := "hostname.localhost"
			tags, expUniq := genTags(size, 2)
			tagsBuf := util.NewHashingTagsBuilderFromSlice(tags)

			generator := NewKeyGenerator()
			expKey := generator.Generate(name, hostname, util.NewHashingTagsBuilderFromSlice(tagsBuf.Copy()))
			for i := 0; i < iterations; i++ {
				tags := tagsBuf.Copy()
				r.Shuffle(size, func(i, j int) { tags[i], tags[j] = tags[j], tags[i] })
				tagsBuf := util.NewHashingTagsBuilderFromSlice(tags)
				key := generator.Generate(name, hostname, tagsBuf)
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

func BenchmarkKeyGeneration(b *testing.B) {
	name := "testname"
	host := "myhost"
	for i := 1; i < 4096; i *= 2 {
		tags, _ := genTags(i, 1)
		tagsBuf := util.NewHashingTagsBuilderFromSlice(tags)
		b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
			generator := NewKeyGenerator()
			tags := util.NewHashingTagsBuilderFromSlice(tagsBuf.Copy())
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				generator.Generate(name, host, tags)
			}
		})

	}
}
