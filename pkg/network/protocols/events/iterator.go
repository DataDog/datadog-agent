// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import "unsafe"

// iterator provides a small abstraction for walking over the the raw stream of
// bytes represented by the `batch.data` field.
// for example, batch data for an event type of size 4 may look like the
// following:
//
// 0    1    2    3    4    5
// |aaaa|bbbb|cccc|0000|0000|....|
// 0    4    8    12   16   20   4096
//
// The purpose of the iterator is to simply return the appropriate chunks of
// data. If we instantiate the iterator with i=1 and j=3, calling Next()
// multiple times will return the following:
//
// Next() => bbbb
// Next() => cccc
// Next() => nil
type iterator struct {
	data []byte
	b    *batch
	i, j int // data offsets
}

// newIterator returns a new `iterator` instance
func newIterator(b *batch, i, j int) *iterator {
	data := unsafe.Slice((*byte)(unsafe.Pointer(&b.Data[0])), batchBufferSize)
	return &iterator{
		data: data,
		b:    b,
		i:    i - 1,
		j:    j,
	}
}

// Next will advance to the next chunk of data representing an eBPF "event"
// while taking into consideration the bounds of the data.
// In case we run out of bounds `nil` is returned
func (it *iterator) Next() []byte {
	it.i++

	chunkSize := int(it.b.Event_size)
	if it.i >= it.j || it.i > int(it.b.Cap) || (it.i+1)*chunkSize > len(it.b.Data) {
		return nil
	}

	return it.data[it.i*chunkSize : (it.i+1)*chunkSize]
}
