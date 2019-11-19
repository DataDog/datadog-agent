// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	assert.Equal(t, uint64(0x5a2b4635eb90c410), firstKey[0])
	assert.Equal(t, uint64(0xe74b94e39f48391e), firstKey[1])

	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key := generator.Generate(name, hostname, tags)
			assert.Equal(t, firstKey, key)
		})
	}

	otherKey := generator.Generate("othername", hostname, tags)
	assert.NotEqual(t, firstKey, otherKey)
	assert.Equal(t, uint64(0xcd3bca32c0520309), otherKey[0])
	assert.Equal(t, uint64(0xfbb533e63ac0d40f), otherKey[1])
}

func TestCompare(t *testing.T) {
	base := ContextKey{uint64(0xcd3bca32c0520309), uint64(0xfbb533e63ac0d40f)}
	veryHigh := ContextKey{uint64(0xff3bca32c0520309), uint64(0xfbb533e63ac0d40f)}
	littleHigh := ContextKey{uint64(0xff3bca32c0520309), uint64(0xfbb533e63ac0d4ff)}
	veryLow := ContextKey{uint64(0x003bca32c0520309), uint64(0xfbb533e63ac0d40f)}

	assert.Equal(t, 0, Compare(base, base))
	assert.Equal(t, 1, Compare(veryHigh, base))
	assert.Equal(t, 1, Compare(littleHigh, base))
	assert.Equal(t, -1, Compare(veryLow, base))
}

// This benchmark is here to make sure we have
// zero heap allocation for ContextKey generation
//
// run with `go test -bench=. -benchmem ./pkg/aggregator/ckey/`
func BenchmarkGenerateNoAlloc(b *testing.B) {
	name := "testname"
	host := "myhost"
	tags := []string{"foo", "bar"}
	generator := NewKeyGenerator()
	for n := 0; n < b.N; n++ {
		generator.Generate(name, host, tags)
	}
}
