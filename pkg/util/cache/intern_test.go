// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type localRetainer struct {
	retentions map[Refcounted]int32
}

func newRetainer() *localRetainer {
	return &localRetainer{
		retentions: make(map[Refcounted]int32),
	}
}

func (r *localRetainer) Reference(obj Refcounted) {
	r.retentions[obj] += 1
}
func (r *localRetainer) ReleaseAllWith(callback func(obj Refcounted, count int32)) {
	for k, v := range r.retentions {
		callback(k, v)
		delete(r.retentions, k)
	}
}

func (r *localRetainer) ReleaseAll() {
	r.ReleaseAllWith(func(obj Refcounted, count int32) {
		interner := obj.(*stringInterner)
		interner.Release(int32(count))
	})
}

func TestInternLoadOrStoreValue(t *testing.T) {
	assert := assert.New(t)
	sInterner, err := NewKeyedStringInterner(3, -1, true)
	retainer := newRetainer()
	assert.NoError(err)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	v := sInterner.LoadOrStore(foo, "", retainer)
	assert.Equal("foo", v)
	v = sInterner.LoadOrStore(bar, "", retainer)
	assert.Equal("bar", v)
	v = sInterner.LoadOrStore(far, "", retainer)
	assert.Equal("far", v)
	v = sInterner.LoadOrStore(boo, "", retainer)
	assert.Equal("boo", v)

	// Now test that the retainer is correct
	assert.Equal(1, len(retainer.retentions))
	for _, v := range retainer.retentions {
		assert.Equal(4, v)
	}
	sInterner.Release(retainer)
	assert.Equal(0, len(retainer.retentions))
}

func TestInternLoadOrStorePointer(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(4, "/tmp")

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

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

func TestInternLoadOrStoreReset(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(4, "/tmp")

	sInterner.loadOrStore([]byte("foo"))
	assert.Equal(1, len(sInterner.cache.strings))
	sInterner.loadOrStore([]byte("bar"))
	sInterner.loadOrStore([]byte("bar"))
	assert.Equal(2, len(sInterner.cache.strings))
	sInterner.loadOrStore([]byte("boo"))
	assert.Equal(3, len(sInterner.cache.strings))
	sInterner.loadOrStore([]byte("far"))
	sInterner.loadOrStore([]byte("far"))
	sInterner.loadOrStore([]byte("far"))
	// Foo is the 4th-least recently used.
	assert.Contains(sInterner.cache.strings, "foo", "first element still in cache")
	assert.Equal(4, len(sInterner.cache.strings))
	sInterner.loadOrStore([]byte("val"))
	// Something got bumped
	assert.Equal(4, len(sInterner.cache.strings))
	// Foo was it.
	assert.NotContains(sInterner.cache.strings, "foo", "oldest element evicted")
	sInterner.loadOrStore([]byte("val"))
	assert.Equal(4, len(sInterner.cache.strings))
}

func NoTestLoadSeveralGenerations(t *testing.T) {
	assert := assert.New(t)
	interner, err := NewKeyedStringInterner(8, -1, true)
	assert.NoError(err)
	retainer := newRetainer()

	// Start generating random strings until we fill a few gigabytes of memory.
	var totalUsed uint64 = 0
	for totalUsed < (64 * 1073741824) {
		text := make([]byte, 64)
		interner.LoadOrStore(text, "", retainer)
		s := string(text)
		totalUsed += uint64(len(s))
	}

	interner.Release(retainer)
}
