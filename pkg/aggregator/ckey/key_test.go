// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, ContextKey(0x932f9848b0fb0802), firstKey)

	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key := generator.Generate(name, hostname, tags)
			assert.Equal(t, firstKey, key)
		})
	}

	otherKey := generator.Generate("othername", hostname, tags)
	assert.NotEqual(t, firstKey, otherKey)
	assert.Equal(t, ContextKey(0xb059e8f73b4b7ae0), otherKey)
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
