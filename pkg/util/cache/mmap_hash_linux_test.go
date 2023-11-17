// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"unsafe"
)

func Test_mmap_hash(t *testing.T) {
	table, err := newMmapHash("", 8192, "/tmp", false)
	assert.NoError(t, err)

	foo, _ := table.lookupOrInsert([]byte("foo"))
	bar, _ := table.lookupOrInsert([]byte("bar"))
	tooLong := make([]byte, 4200) // larger than 4096
	for i := range tooLong {
		tooLong[i] = byte(i % 256)
	}

	foo2, _ := table.lookupOrInsert([]byte("foo"))
	bar2, _ := table.lookupOrInsert([]byte("bar"))
	fmt.Printf("foo: %p, foo2: %p, bar: %p, bar2: %p\n",
		unsafe.StringData(foo), unsafe.StringData(foo2),
		unsafe.StringData(bar), unsafe.StringData(bar2))
	assert.Equal(t, unsafe.StringData(foo), unsafe.StringData(foo2))
	assert.Equal(t, unsafe.StringData(bar), unsafe.StringData(bar2))
	_, failed := table.lookupOrInsert(tooLong)
	assert.False(t, failed)
	baz, _ := table.lookupOrInsert([]byte("baz"))
	assert.NotNil(t, baz)
	baz2, _ := table.lookupOrInsert([]byte("baz"))
	assert.Equal(t, unsafe.StringData(baz), unsafe.StringData(baz2))

}
