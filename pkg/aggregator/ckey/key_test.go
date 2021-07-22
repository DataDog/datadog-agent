// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsZero(t *testing.T) {
	var k ContextKey
	assert.True(t, k.IsZero())
}

func TestGenerateReproductible(t *testing.T) {
	name := "metric.name"
	hostname := "hostname"
	tags := []string{"bar", "foo", "key:value", "key:value2"}

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
	key := generator.Generate(name, hostname, tags)

	// change tags order, the generated key should be the same
	tags[0], tags[1], tags[2], tags[3] = tags[3], tags[0], tags[1], tags[2]
	key2 := generator.Generate(name, hostname, tags)
	assert.Equal(key, key2, "order of tags should not matter")

	// add a duplicated tag
	tags = append(tags, "key:value", "foo")
	key3 := generator.Generate(name, hostname, tags)
	assert.Equal(key, key3, "duplicated tags should not matter")

	// and now, completely change of the tag, the generated key should NOT be the same
	tags[2] = "another:tag"
	key4 := generator.Generate(name, hostname, tags)
	assert.NotEqual(key, key4, "tags content should matter")
}

func genTags(count int) []string {
	var tags []string
	for i := 0; i < count; i++ {
		tags = append(tags, fmt.Sprintf("tag%d:value%d", i, i))
	}
	return tags
}

func BenchmarkKeyGeneration(b *testing.B) {
	name := "testname"
	host := "myhost"
	for i := 1; i < 256; i *= 2 {
		b.Run(fmt.Sprintf("%d-tags", i), func(b *testing.B) {
			generator := NewKeyGenerator()
			tags := genTags(i)
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				generator.Generate(name, host, tags)
			}
		})

	}
}
