// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

const (
	eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
	dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
)

var finishedIterating = errors.New("no data items left to iterate")

type dataItem struct {
	header *output.DataItemHeader
	data   []byte
}

func init() {
	if dataItemHeaderSize == 0 || eventHeaderSize == 0 {
		panic("invalid header size for decoding buffers")
	}
}

func advanceBuffer(idx int, size int) int {
	return (idx + size + 7) & ^7 // pad to nearest multiple of 8
}

type Event []byte

func (e Event) eventHeader() (*output.EventHeader, error) {
	if e == nil {
		return nil, errors.New("nil iterator")
	}
	if len(e) < eventHeaderSize {
		return nil, errors.New("not enough bytes to read event header")
	}
	return (*output.EventHeader)(unsafe.Pointer(&e[0])), nil
}

func (e Event) dataItems() iter.Seq2[dataItem, error] {
	return func(yield func(dataItem, error) bool) {
		eventHeader, err := e.eventHeader()
		if err != nil {
			yield(dataItem{}, err)
			return
		}

		idx := eventHeaderSize // Skip event header
		if len(e) < idx+int(eventHeader.Stack_byte_len) {
			yield(dataItem{}, fmt.Errorf("not enough bytes to read stack trace: %d < %d", len(e), idx+int(eventHeader.Stack_byte_len)))
			return
		}
		idx = advanceBuffer(idx, int(eventHeader.Stack_byte_len)) // Skip stack trace data
		for {
			if idx+dataItemHeaderSize >= len(e) {
				yield(dataItem{}, fmt.Errorf("not enough bytes to read data item header: %d < %d", len(e), idx+dataItemHeaderSize))
				return
			}
			header := (*output.DataItemHeader)(unsafe.Pointer(&e[idx]))
			idx = advanceBuffer(idx, dataItemHeaderSize)

			if idx+int(header.Length) > len(e) {
				yield(dataItem{}, fmt.Errorf("not enough bytes to read data item: %d < %d", len(e), idx+int(header.Length)))
				return
			}
			data := e[idx : idx+int(header.Length)]

			idx = advanceBuffer(idx, int(header.Length))
			item := dataItem{
				header: header,
				data:   data,
			}
			if !yield(item, nil) {
				return
			}
		}
	}
}

func (e Event) stackFrames() ([]uint64, error) {
	eventHeader, err := e.eventHeader()
	if err != nil {
		return nil, err
	}
	idx := eventHeaderSize
	stackTraceLen := int(eventHeader.Stack_byte_len)
	if len(e) < idx+stackTraceLen {
		return nil, errors.New("not enough bytes to read stack trace")
	}

	// Each stack frame is 8 bytes (uint64)
	stackData := e[idx : idx+stackTraceLen]
	numFrames := stackTraceLen / 8
	if stackTraceLen%8 != 0 {
		return nil, errors.New("stack trace length is not a multiple of 8 bytes")
	}
	frames := make([]uint64, numFrames)
	for i := range numFrames {
		framePtr := (*uint64)(unsafe.Pointer(&stackData[i*8]))
		frames[i] = *framePtr
	}
	return frames, nil
}
