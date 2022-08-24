// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestIsZero(t *testing.T) {
	var k ContextKey
	assert.True(t, k.IsZero())
}

func TestGenerateReproductible(t *testing.T) {
	name := "metric.name"
	hostname := "hostname"
	tags := tagset.NewHashingTagsAccumulatorWithTags([]string{"bar", "foo", "key:value", "key:value2"})

	generator := NewKeyGenerator()

	firstKey := generator.Generate(name, hostname, tags)
	assert.Equal(t, ContextKey(0x1e923504c1aad3ad), firstKey)

	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key := generator.Generate(name, hostname, tags)
			assert.Equal(t, firstKey, key)
		})
	}

	otherKey := generator.Generate("othername", hostname, tags)
	assert.NotEqual(t, firstKey, otherKey)
	assert.Equal(t, ContextKey(0xd298ae9740130f30), otherKey)
}

func TestGenerateReproductible2(t *testing.T) {
	name := "metric.name"
	hostname := "hostname"
	tags1 := tagset.NewHashingTagsAccumulatorWithTags([]string{"bar", "foo", "key:value", "key:value2"})
	tags2 := tagset.NewHashingTagsAccumulatorWithTags([]string{})

	generator := NewKeyGenerator()

	firstKey, tagsKey1, tagsKey2 := generator.GenerateWithTags2(name, hostname, tags1, tags2)
	assert.Equal(t, ContextKey(0x1e923504c1aad3ad), firstKey)
	assert.Equal(t, TagsKey(0x437b13a371a1c7d3), tagsKey1)
	assert.Equal(t, TagsKey(0), tagsKey2)

	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key, t1, t2 := generator.GenerateWithTags2(name, hostname, tags1, tags2)
			assert.Equal(t, firstKey, key)
			assert.Equal(t, tagsKey1, t1)
			assert.Equal(t, tagsKey2, t2)
		})
	}

	otherKey, otherTagsKey1, otherTagsKey2 := generator.GenerateWithTags2("othername", hostname, tags1, tags2)
	assert.NotEqual(t, firstKey, otherKey)
	assert.Equal(t, ContextKey(0xd298ae9740130f30), otherKey)
	assert.Equal(t, tagsKey1, otherTagsKey1)
	assert.Equal(t, tagsKey2, otherTagsKey2)
}

func TestMetricTagOverlap(t *testing.T) {
	g := NewKeyGenerator()

	empty := tagset.NewHashingTagsAccumulator()
	h1, _, _ := g.GenerateWithTags2("metric1", "hostname",
		tagset.NewHashingTagsAccumulatorWithTags([]string{"metric1", "t1", "t2"}), empty)
	h2, _, _ := g.GenerateWithTags2("metric2", "hostname",
		tagset.NewHashingTagsAccumulatorWithTags([]string{"metric2", "t1", "t2"}), empty)

	assert.NotEqual(t, h1, h2)
}

func TestMetricHostnameSplit(t *testing.T) {
	g := NewKeyGenerator()
	empty := tagset.NewHashingTagsAccumulator()
	h1, _, _ := g.GenerateWithTags2("metric", "hostname", empty, empty)
	h2, _, _ := g.GenerateWithTags2("metrichost", "name", empty, empty)

	assert.NotEqual(t, h1, h2)
}

func TestCompare(t *testing.T) {
	base := ContextKey(uint64(0xff3bca32c0520309))
	same := ContextKey(uint64(0xff3bca32c0520309))
	diff := ContextKey(uint64(0xcd3bca32c0520309))

	assert.True(t, Equals(base, base))
	assert.True(t, Equals(base, same))
	assert.False(t, Equals(base, diff))
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

func BenchmarkKeyGeneration(b *testing.B) {
	name := "testname"
	host := "myhost"
	for i := 1; i < 4096; i *= 2 {
		tags, _ := genTags(i, 1)
		tagsBuf := tagset.NewHashingTagsAccumulatorWithTags(tags)
		b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
			generator := NewKeyGenerator()
			tags := tagsBuf.Dup()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				generator.Generate(name, host, tags)
			}
		})

	}
}

func BenchmarkKeyGeneration2(b *testing.B) {
	name := "testname"
	host := "myhost"

	variant := os.Getenv("VARIANT")
	for i := 1; i < 4096; i *= 2 {
		tags, _ := genTags(i, 1)
		if variant == "2" {
			l := tagset.NewHashingTagsAccumulatorWithTags(tags[:i/2])
			r := tagset.NewHashingTagsAccumulatorWithTags(tags[i/2:])
			b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
				generator := NewKeyGenerator()
				l := l.Dup()
				r := r.Dup()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					generator.GenerateWithTags2(name, host, l, r)
				}
			})
		} else {
			tagsBuf := tagset.NewHashingTagsAccumulatorWithTags(tags)
			b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
				generator := NewKeyGenerator()
				tags := tagsBuf.Dup()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					generator.GenerateWithTags(name, host, tags)
				}
			})
		}
	}
}
