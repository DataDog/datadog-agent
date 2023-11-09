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

func (r *localRetainer) ReferenceN(obj Refcounted, n int32) {
	//TODO implement me
	panic("implement me")
}

func (r *localRetainer) CopyTo(dest InternRetainer) {
	//TODO implement me
	panic("implement me")
}

func (r *localRetainer) Import(source InternRetainer) {
	//TODO implement me
	panic("implement me")
}

func newRetainer() *localRetainer {
	return &localRetainer{
		retentions: make(map[Refcounted]int32),
	}
}

func (r *localRetainer) Reference(obj Refcounted) {
	r.retentions[obj]++
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
	sInterner := NewKeyedStringInternerMemOnly(3)
	retainer := newRetainer()

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
		assert.Equal(4, int(v))
	}
	retainer.ReleaseAll()
	assert.Equal(0, len(retainer.retentions))
}

func TestInternLoadOrStorePointer(t *testing.T) {
	assert := assert.New(t)
	sInterner := NewKeyedStringInternerMemOnly(4)
	retainer := newRetainer()

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	v := sInterner.loadOrStore(foo, "", retainer)
	assert.Equal("foo", v)
	v2 := sInterner.loadOrStore(foo, "", retainer)
	assert.Equal(&v, &v2, "must point to the same address")
	v2 = sInterner.loadOrStore(bar, "", retainer)
	assert.NotEqual(&v, &v2, "must point to a different address")
	v3 := sInterner.loadOrStore(bar, "", retainer)
	assert.Equal(&v2, &v3, "must point to the same address")

	v4 := sInterner.loadOrStore(boo, "", retainer)
	assert.NotEqual(&v, &v4, "must point to a different address")
	assert.NotEqual(&v2, &v4, "must point to a different address")
	assert.NotEqual(&v3, &v4, "must point to a different address")
}

func TestInternLoadOrStoreReset(t *testing.T) {
	assert := assert.New(t)
	sInterner := NewKeyedStringInternerMemOnly(4)
	retainer := newRetainer()
	cacheLen := func() int {
		internerUntyped, ok := sInterner.interners.Get("")
		if !ok {
			return 0
		}
		interner := internerUntyped.(*stringInterner)
		return len(interner.cache.strings)
	}
	assertCacheContains := func(s, comment string) {
		internerUntyped, ok := sInterner.interners.Get("")
		if !ok {
			assert.Fail("No interner to hold key: " + comment)
		}
		interner := internerUntyped.(*stringInterner)
		assert.Contains(interner.cache.strings, s, comment)
	}

	sInterner.loadOrStore([]byte("foo"), "", retainer)
	assert.Equal(1, cacheLen())
	sInterner.loadOrStore([]byte("bar"), "", retainer)
	sInterner.loadOrStore([]byte("bar"), "", retainer)
	assert.Equal(2, cacheLen())
	sInterner.loadOrStore([]byte("boo"), "", retainer)
	assert.Equal(3, cacheLen())
	sInterner.loadOrStore([]byte("far"), "", retainer)
	sInterner.loadOrStore([]byte("far"), "", retainer)
	sInterner.loadOrStore([]byte("far"), "", retainer)
	// Foo is the 4th-least recently used.
	assertCacheContains("foo", "first element still in cache")
	assert.Equal(4, cacheLen())
	sInterner.loadOrStore([]byte("val"), "", retainer)
	// Something got bumped
	assert.Equal(4, cacheLen())
	// Foo was it.
	assertCacheContains("foo", "oldest element evicted")
	sInterner.loadOrStore([]byte("val"), "", retainer)
	assert.Equal(4, cacheLen())
}

func NoTestLoadSeveralGenerations(t *testing.T) {
	interner := NewKeyedStringInternerMemOnly(8)
	retainer := newRetainer()

	// Start generating random strings until we fill a few gigabytes of memory.
	var totalUsed uint64
	for totalUsed < (64 * 1073741824) {
		text := make([]byte, 64)
		interner.LoadOrStore(text, "", retainer)
		s := string(text)
		totalUsed += uint64(len(s))
	}

	retainer.ReleaseAll()
}
