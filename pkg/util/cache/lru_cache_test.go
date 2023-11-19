// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"unsafe"
)

func heapAlloc(key []byte) string {
	return string(key)
}

func Test_lruStringCache_deduplicate(t *testing.T) {
	table := newLruStringCache(4, false)
	assert.Empty(t, table.strings)

	foo := table.lookupOrInsert([]byte("foo"), heapAlloc)
	bar := table.lookupOrInsert([]byte("bar"), heapAlloc)
	foo2 := table.lookupOrInsert([]byte("foo"), heapAlloc)
	bar2 := table.lookupOrInsert([]byte("bar"), heapAlloc)

	assert.Equal(t, table.head.s, "bar")
	assert.Equal(t, table.tail.s, "foo")
	assert.Equal(t, unsafe.StringData(foo), unsafe.StringData(foo2))
	assert.Equal(t, unsafe.StringData(bar), unsafe.StringData(bar2))

}

func Test_lruStringCache_evicts(t *testing.T) {
	table := newLruStringCache(1, false)
	assert.Empty(t, table.strings)

	foo := table.lookupOrInsert([]byte("foo"), heapAlloc)
	assert.Equal(t, table.head.s, "foo")
	assert.Equal(t, table.tail.s, "foo")
	assert.Contains(t, table.strings, foo)

	bar := table.lookupOrInsert([]byte("bar"), heapAlloc)
	bar2 := table.lookupOrInsert([]byte("bar"), heapAlloc)

	assert.Equal(t, table.head.s, "bar")
	assert.Equal(t, table.tail.s, "bar")
	assert.Equal(t, unsafe.StringData(bar), unsafe.StringData(bar2))

	assert.Equal(t, len(table.strings), 1)
	assert.NotContains(t, table.strings, foo)
}
