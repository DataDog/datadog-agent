// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"
)

const (
	eventHeaderSize    = int(unsafe.Sizeof(EventHeader{}))
	dataItemHeaderSize = int(unsafe.Sizeof(DataItemHeader{}))
)

// DataItem represents a single data item in an event.
type DataItem struct {
	header *DataItemHeader
	data   []byte
}

// Header returns the header of the data item.
func (d *DataItem) Header() *DataItemHeader {
	return d.header
}

// Data returns the data of the data item.
func (d *DataItem) Data() []byte {
	return d.data
}

func init() {
	if dataItemHeaderSize == 0 || eventHeaderSize == 0 {
		panic("invalid header size for decoding buffers")
	}
}

func advanceBuffer(idx int, size int) int {
	return (idx + size + 7) & ^7 // pad to nearest multiple of 8
}

// Event represents a single from the BPF program.
type Event []byte

// Header decodes and returns the header of the event.
func (e Event) Header() (*EventHeader, error) {
	if e == nil {
		return nil, errors.New("nil iterator")
	}
	if len(e) < eventHeaderSize {
		return nil, errors.New("not enough bytes to read event header")
	}
	return (*EventHeader)(unsafe.Pointer(&e[0])), nil
}

// DataItems is an iterator over the data items in the event.
func (e Event) DataItems() iter.Seq2[DataItem, error] {
	return func(yield func(DataItem, error) bool) {
		header, err := e.Header()
		if err != nil {
			yield(DataItem{}, err)
			return
		}

		idx := eventHeaderSize // skip event header
		if len(e) < idx+int(header.Stack_byte_len) {
			yield(DataItem{}, fmt.Errorf("not enough bytes to read stack trace: %d < %d",
				len(e), idx+int(header.Stack_byte_len)))
			return
		}
		idx = advanceBuffer(idx, int(header.Stack_byte_len)) // skip stack trace data
		for {
			if idx == len(e) {
				return
			}
			if idx+dataItemHeaderSize >= len(e) {
				yield(DataItem{}, fmt.Errorf("not enough bytes to read data item header: %d > %d",
					idx+dataItemHeaderSize, len(e)))
				return
			}
			header := (*DataItemHeader)(unsafe.Pointer(&e[idx]))
			idx = advanceBuffer(idx, dataItemHeaderSize)
			dataLen := int(header.Length)
			if idx+dataLen > len(e) {
				yield(DataItem{}, fmt.Errorf("not enough bytes to read data item: %d < %d",
					len(e), idx+int(header.Length)))
				return
			}
			data := e[idx : idx+dataLen]

			idx = advanceBuffer(idx, dataLen)
			item := DataItem{
				header: header,
				data:   data,
			}
			if !yield(item, nil) {
				return
			}
		}
	}
}

// StackPCs decodes the program counters of the stack trace from the event.
func (e Event) StackPCs() ([]uint64, error) {
	header, err := e.Header()
	if err != nil {
		return nil, err
	}
	idx := eventHeaderSize
	stackTraceLen := int(header.Stack_byte_len)
	if stackTraceLen%8 != 0 {
		return nil, errors.New("stack trace length is not a multiple of 8 bytes")
	}
	if len(e) < idx+stackTraceLen {
		return nil, errors.New("not enough bytes to read stack trace")
	}
	stackData := e[idx : idx+stackTraceLen]
	numFrames := stackTraceLen / 8
	frames := unsafe.Slice((*uint64)(unsafe.Pointer(&stackData[0])), numFrames)
	return frames, nil
}
