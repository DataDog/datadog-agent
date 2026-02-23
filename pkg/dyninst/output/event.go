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

// EventPairingExpectation returns the event pairing expectation.
type EventPairingExpectation uint8

// This must be kept in sync with the event_pairing_expectation enum in the
// ebpf/framing.h file.
const (
	EventPairingExpectationNone                  EventPairingExpectation = 0
	EventPairingExpectationEntryPairingExpected  EventPairingExpectation = 1
	EventPairingExpectationReturnPairingExpected EventPairingExpectation = 2
	EventPairingExpectationCallCountExceeded     EventPairingExpectation = 3
	EventPairingExpectationCallMapFull           EventPairingExpectation = 4
	EventPairingExpectationBufferFull            EventPairingExpectation = 5
	EventPairingExpectationNoneInlined           EventPairingExpectation = 6
	EventPairingExpectationNoneNoBody            EventPairingExpectation = 7
)

const (
	// DataItemFailedReadMask is a mask on the type field of a data item header that
	// can be used to check if a data item was marked as a failed read.
	DataItemFailedReadMask = uint32(1 << 31)
	// DataItemTypeMask is a mask on the type field of a data item header that can be
	// used to get the type of a data item without the failed read mask.
	DataItemTypeMask = ^DataItemFailedReadMask
)

// DataItem represents a single data item in an event.
type DataItem struct {
	header *DataItemHeader
	data   []byte
}

// IsFailedRead returns true if the data item was marked as a failed read.
func (d *DataItem) IsFailedRead() bool {
	return d.header.Type&DataItemFailedReadMask != 0
}

// Type returns the type of the data item without the failed read mask.
func (d *DataItem) Type() uint32 {
	return d.header.Type & DataItemTypeMask
}

// Header returns the header of the data item.
func (d *DataItem) Header() *DataItemHeader {
	return d.header
}

// Data returns the data of the data item if it was not marked as a failed read.
func (d *DataItem) Data() ([]byte, bool) {
	if d.header.Type&DataItemFailedReadMask != 0 {
		return nil, false
	}
	return d.data, true
}

func nextMultipleOf8(v int) int {
	return (v + 7) & ^7 // pad to nearest multiple of 8
}

// Event represents a single event from the BPF program.
type Event []byte

var errNoDataItems = errors.New("no data items found")

// FirstDataItemHeader returns the header of the first data item in the event.
func (e Event) FirstDataItemHeader() (*DataItemHeader, error) {
	var item DataItem
	err := errNoDataItems
	for item, err = range e.DataItems() {
		break
	}
	return item.header, err
}

// Header decodes and returns the header of the event.
//
// It verifies that the event data is well-formed, i.e. that the header is
// aligned to 8 bytes and that the data length is consistent with the event
// size.
func (e Event) Header() (*EventHeader, error) {
	if len(e) < eventHeaderSize {
		return nil, fmt.Errorf(
			"not enough bytes to read event header: %d < %d",
			len(e), eventHeaderSize,
		)
	}

	// It's not strictly necessary to check this alignment as on x86 and on
	// modern ARM machines, unaligned accesses are okay. That being said, it
	// seems very unlikely that non-buggy code would ever provide event data
	// that is not aligned to 8 bytes. The Go allocator is always going to
	// allocate chunks that are at least 8 byte aligned [0]. If we ever are to
	// pull the data directly from a mmaped ringbuffer, there too we know that
	// all events are guaranteed to be 8 byte aligned.
	//
	// [0] https://github.com/golang/go/blob/456a90aa/src/internal/runtime/gc/sizeclasses.go
	if uintptr(unsafe.Pointer(&e[0]))%8 != 0 {
		return nil, fmt.Errorf(
			"event data is not aligned to 8 bytes: %p",
			unsafe.Pointer(&e[0]),
		)
	}

	h := (*EventHeader)(unsafe.Pointer(&e[0]))
	if h.Data_byte_len != uint32(len(e)) {
		return nil, fmt.Errorf(
			"event length mismatch: %d != %d",
			h.Data_byte_len, len(e),
		)
	}
	return h, nil
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
			yield(DataItem{}, fmt.Errorf(
				"not enough bytes to read stack trace: %d < %d",
				len(e), idx+int(header.Stack_byte_len),
			))
			return
		}
		if header.Stack_byte_len%8 != 0 {
			yield(DataItem{}, fmt.Errorf(
				"stack trace length is not a multiple of 8 bytes: %d",
				header.Stack_byte_len,
			))
			return
		}
		idx += int(header.Stack_byte_len) // skip stack trace data, aligned by construction
		for {
			if idx == len(e) {
				return
			}
			if idx+dataItemHeaderSize > len(e) {
				yield(DataItem{}, fmt.Errorf(
					"not enough bytes to read data item header: %d > %d",
					idx+dataItemHeaderSize, len(e),
				))
				return
			}
			header := (*DataItemHeader)(unsafe.Pointer(&e[idx]))
			idx += dataItemHeaderSize // known to be aligned to 8 bytes
			dataLen := int(header.Length)
			if idx+dataLen > len(e) {
				yield(DataItem{}, fmt.Errorf(
					"not enough bytes to read data item (%d bytes): %d < %d",
					header.Length, len(e), idx+int(header.Length),
				))
				return
			}
			data := e[idx : idx+dataLen : idx+dataLen]
			idx = nextMultipleOf8(idx + dataLen)
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
	if numFrames == 0 {
		return nil, nil
	}
	frames := unsafe.Slice((*uint64)(unsafe.Pointer(&stackData[0])), numFrames)
	return frames, nil
}
