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

	firstKey := Generate(name, hostname, tags)
	assert.Equal(t, "10c490eb35462b5a1e39489fe3944be7", firstKey.String())

	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			key := Generate(name, hostname, tags)
			assert.Equal(t, firstKey, key)
		})
	}

	otherKey := Generate("othername", hostname, tags)
	assert.NotEqual(t, firstKey, otherKey)
	assert.Equal(t, "cd3bca32c0520309fbb533e63ac0d40f", otherKey.String())
}

func TestCompare(t *testing.T) {
	base, _ := Parse("cd3bca32c0520309fbb533e63ac0d40f")
	veryHigh, _ := Parse("ff3bca32c0520309fbb533e63ac0d40f")
	littleHigh, _ := Parse("ff3bca32c0520309fbb533e63ac0d4ff")
	veryLow, _ := Parse("003bca32c0520309fbb533e63ac0d40f")

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
	for n := 0; n < b.N; n++ {
		Generate(name, host, tags)
	}
}
