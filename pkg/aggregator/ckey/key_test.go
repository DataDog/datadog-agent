// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func compareWithLen(left, right string) bool {
	if len(left) != len(right) || left != right {
		return false
	}
	return true
}

func compareWithoutLen(left, right string) bool {
	if left != right {
		return false
	}
	return true
}

func BenchmarkCompareWith(b *testing.B) {
	for i := 1; i < 256; i *= 2 {
		b.Run(fmt.Sprintf("%d-strings", i), func(b *testing.B) {
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				word1 := strconv.Itoa(rand.Int())
				word2 := strconv.Itoa(rand.Int())
				compareWithLen(word1, word2)
			}
		})
	}
}

func BenchmarkCompareWithout(b *testing.B) {
	for i := 1; i < 256; i *= 2 {
		b.Run(fmt.Sprintf("%d-strings", i), func(b *testing.B) {
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				word1 := strconv.Itoa(rand.Int())
				word2 := strconv.Itoa(rand.Int())
				compareWithoutLen(word1, word2)
			}
		})
	}
}

func TestIsZero(t *testing.T) {
	var k ContextKey
	assert.True(t, k.IsZero())
}

/*
func TestGenerateReproductible(t *testing.T) {
	assert := assert.New(t)

	name := "metric.name"
	hostname := "hostname"
	tags := []string{"bar", "foo", "key:value", "key:value2"}

	generator := NewKeyGenerator()

	firstKey := generator.Generate(name, hostname, tags)
	assert.Equal(ContextKey(0x3777172ebed84057), firstKey)

	// test that multiple generations with the same generator
	// will lead to the same hash while providing the same (name, hostname, tags)
	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key := generator.Generate(name, hostname, tags)
			assert.Equal(firstKey, key)
		})
	}

	// now that a generation with the same generator is providing a different hash
	// for a different sets of (name, hostname, tags)

	otherKey := generator.Generate("othername", hostname, tags)
	assert.NotEqual(firstKey, otherKey)
	assert.Equal(ContextKey(0x74fe7cd2f068d669), otherKey)

	otherKey = generator.Generate("othername", "anotherhostname", tags)
	assert.NotEqual(firstKey, otherKey)
	assert.Equal(ContextKey(0xaff3301bf26fa7c5), otherKey)

	tags = append(tags, "dev:meh")
	otherKey = generator.Generate("othername", "anotherhostname", tags)
	assert.NotEqual(firstKey, otherKey)
	assert.Equal(ContextKey(0x21e67ef8623423ad), otherKey)
}

func TestTagsOrderDoesntMatter(t *testing.T) {
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

	// and now, completely change of the tag, the generated key should NOT be the same
	tags[2] = "another:tag"
	key3 := generator.Generate(name, hostname, tags)
	assert.NotEqual(key, key3, "tags content should matter")
}

func TestCompare(t *testing.T) {
	base := ContextKey(uint64(0xff3bca32c0520309))
	same := ContextKey(uint64(0xff3bca32c0520309))
	diff := ContextKey(uint64(0xcd3bca32c0520309))

	assert.True(t, Equals(base, base))
	assert.True(t, Equals(base, same))
	assert.False(t, Equals(base, diff))
}
*/

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
	for i := 1; i < 128; i += 2 {
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
