// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package events

import "unsafe"

type iterator struct {
	data []byte
	b    *batch
	i, j int // data offsets
}

func newIterator(b *batch, i, j int) *iterator {
	// TODO: figure out how to create a byte slice without allocating
	data := *(*[batchBufferSize]byte)(unsafe.Pointer(&b.Data[0]))
	return &iterator{
		data: data[:],
		b:    b,
		i:    i - 1,
		j:    j,
	}
}

func (it *iterator) Next() []byte {
	it.i++

	chunkSize := int(it.b.Size)
	if it.i >= it.j || it.i > int(it.b.Cap) || (it.i+1)*chunkSize > len(it.b.Data) {
		return nil
	}

	return it.data[it.i*chunkSize : (it.i+1)*chunkSize]
}
