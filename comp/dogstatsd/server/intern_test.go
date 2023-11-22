// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkloadOrStoreReset(b *testing.B) {
	sInterner := newStringInterner(4, 1)

	// benchmark with the internal telemetry enabled
	sInterner.telemetry.enabled = true
	sInterner.prepareTelemetry()

	list := []string{}
	for i := 0; i < 512; i++ {
		list = append(list, fmt.Sprintf("testing.metric%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sInterner.loadOrStore([]byte(list[i%len(list)]))
	}
}

func TestInternloadOrStoreValue(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(3, 1)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.loadOrStore(foo)
	assert.Equal("foo", v)
	v = sInterner.loadOrStore(bar)
	assert.Equal("bar", v)
	v = sInterner.loadOrStore(far)
	assert.Equal("far", v)
	v = sInterner.loadOrStore(boo)
	assert.Equal("boo", v)
}

func TestInternloadOrStorePointer(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(4, 1)

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.loadOrStore(foo)
	assert.Equal("foo", v)
	v2 := sInterner.loadOrStore(foo)
	assert.Equal(&v, &v2, "must point to the same address")
	v2 = sInterner.loadOrStore(bar)
	assert.NotEqual(&v, &v2, "must point to a different address")
	v3 := sInterner.loadOrStore(bar)
	assert.Equal(&v2, &v3, "must point to the same address")

	v4 := sInterner.loadOrStore(boo)
	assert.NotEqual(&v, &v4, "must point to a different address")
	assert.NotEqual(&v2, &v4, "must point to a different address")
	assert.NotEqual(&v3, &v4, "must point to a different address")
}

func TestInternloadOrStoreReset(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(4, 1)

	// first test that the good value is returned.
	sInterner.loadOrStore([]byte("foo"))
	assert.Equal(1, len(sInterner.strings))
	sInterner.loadOrStore([]byte("bar"))
	sInterner.loadOrStore([]byte("bar"))
	assert.Equal(2, len(sInterner.strings))
	sInterner.loadOrStore([]byte("boo"))
	assert.Equal(3, len(sInterner.strings))
	sInterner.loadOrStore([]byte("far"))
	sInterner.loadOrStore([]byte("far"))
	sInterner.loadOrStore([]byte("far"))
	assert.Equal(4, len(sInterner.strings))
	sInterner.loadOrStore([]byte("val"))
	assert.Equal(1, len(sInterner.strings))
	sInterner.loadOrStore([]byte("val"))
	assert.Equal(1, len(sInterner.strings))
}
