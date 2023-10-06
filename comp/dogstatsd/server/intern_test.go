// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternLoadOrStoreValue(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(1)

	foo := []byte("foo")
	bar := []byte("bar")
	far := []byte("far")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo).Get()
	assert.Equal("foo", v)
	v = sInterner.LoadOrStore(bar).Get()
	assert.Equal("bar", v)
	v = sInterner.LoadOrStore(far).Get()
	assert.Equal("far", v)
	v = sInterner.LoadOrStore(boo).Get()
	assert.Equal("boo", v)
}

func TestInternLoadOrStorePointer(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(1)

	foo := []byte("foo")
	bar := []byte("bar")
	boo := []byte("boo")

	// first test that the good value is returned.

	v := sInterner.LoadOrStore(foo).Get()
	assert.Equal("foo", v)
	v2 := sInterner.LoadOrStore(foo).Get()
	assert.Equal(&v, &v2, "must point to the same address")
	v2 = sInterner.LoadOrStore(bar).Get()
	assert.NotEqual(&v, &v2, "must point to a different address")
	v3 := sInterner.LoadOrStore(bar).Get()
	assert.Equal(&v2, &v3, "must point to the same address")

	v4 := sInterner.LoadOrStore(boo).Get()
	assert.NotEqual(&v, &v4, "must point to a different address")
	assert.NotEqual(&v2, &v4, "must point to a different address")
	assert.NotEqual(&v3, &v4, "must point to a different address")
}

func TestInternLoadOrStoreReset(t *testing.T) {
	assert := assert.New(t)
	sInterner := newStringInterner(1)

	// first test that the good value is returned.
	sInterner.LoadOrStore([]byte("foo"))
	assert.Equal(1, len(sInterner.valMap))
	sInterner.LoadOrStore([]byte("bar"))
	sInterner.LoadOrStore([]byte("bar"))
	assert.Equal(2, len(sInterner.valMap))
	sInterner.LoadOrStore([]byte("boo"))
	assert.Equal(3, len(sInterner.valMap))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	assert.Equal(4, len(sInterner.valMap))
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(5, len(sInterner.valMap))
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(5, len(sInterner.valMap))
}
