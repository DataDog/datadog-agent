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

func BenchmarkLoadOrStoreReset(b *testing.B) {
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
		sInterner.LoadOrStore([]byte(list[i%len(list)]))
	}
}

func TestInternLoadOrStoreValue(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(3, 1)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v)
	v = sInterner.LoadOrStore(bar)
	assert.Equal("bar", v)
	v = sInterner.LoadOrStore(far)
	assert.Equal("far", v)
	v = sInterner.LoadOrStore(boo)
	assert.Equal("boo", v)
}

func TestInternLoadOrStorePointer(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(4, 1)

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo)
	assert.Equal("foo", v)
	v2 := sInterner.LoadOrStore(foo)
	assert.Equal(&v, &v2, "must point to the same address")
	v2 = sInterner.LoadOrStore(bar)
	assert.NotEqual(&v, &v2, "must point to a different address")
	v3 := sInterner.LoadOrStore(bar)
	assert.Equal(&v2, &v3, "must point to the same address")

	v4 := sInterner.LoadOrStore(boo)
	assert.NotEqual(&v, &v4, "must point to a different address")
	assert.NotEqual(&v2, &v4, "must point to a different address")
	assert.NotEqual(&v3, &v4, "must point to a different address")
}

func TestInternLoadOrStoreReset(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(4, 1)

	// first test that the good value is returned.
	sInterner.LoadOrStore([]byte("foo"))
	assert.Equal(1, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("bar"))
	sInterner.LoadOrStore([]byte("bar"))
	assert.Equal(2, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("boo"))
	assert.Equal(3, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	assert.Equal(4, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(1, len(sInterner.strings))
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(1, len(sInterner.strings))
}
