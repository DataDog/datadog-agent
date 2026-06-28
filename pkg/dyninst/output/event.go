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

// MaxDataItemSize is the maximum number of payload bytes the BPF
// scratch buffer's serialize_whole dispatcher can emit in a single
// data item (i.e., the largest entry in scratch.h's SIZE_LIST). The
// BPF stack machine clamps to this value before serialization so an
// oversized configured maxLength produces a truncated capture rather
// than a silent skip. Userspace can use this constant to surface a
// diagnostic when a probe's maxLength would be clamped at the BPF
// boundary. Must be kept in sync with MAX_DATA_ITEM_SIZE in
// pkg/dyninst/ebpf/scratch.h.
const MaxDataItemSize = 8192

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
	EventPairingExpectationConditionFailed       EventPairingExpectation = 8
	// EventPairingExpectationReturnPanicUnwound stamps a synthetic return
	// event emitted by the runtime.recovery uprobe when a probed frame is
	// unwound by panic+recover and would otherwise leak its in-progress
	// pairing state (BPF in_progress_calls slot and userspace
	// bufferedEvent). The synthetic event carries the panic value in place
	// of return captures.
	EventPairingExpectationReturnPanicUnwound EventPairingExpectation = 9
)

// The DataItemHeader.Type field is packed:
//   - bit 31 (DataItemFailedReadMask): payload absent (kernel read failed,
//     or SM emitted a placeholder for an omitted value with length == 0).
//   - bits 27..30 (DataItemReasonMask): a 4-bit DataItemReason describing
//     why the item is incomplete (placeholder) or truncated (real item).
//     Zero = no reason set; 15 reserved for a future side-channel
//     mechanism.
//   - bits 0..26 (DataItemTypeMask): the type ID.
//
// Kept in sync with pkg/dyninst/ebpf/framing.h.
const (
	// DataItemFailedReadMask is bit 31 on the type field of a data item
	// header. Set when the payload is absent.
	DataItemFailedReadMask = uint32(1 << 31)

	// DataItemReasonShift / DataItemReasonMask cover the 4-bit reason
	// field in bits 27..30.
	DataItemReasonShift = 27
	DataItemReasonMask  = uint32(0xF) << DataItemReasonShift

	// DataItemTypeMask masks out both the failed-read bit and the reason
	// bits, leaving only the type ID.
	DataItemTypeMask = ^(DataItemFailedReadMask | DataItemReasonMask)
)

// DataItemReason classifies *why* a data item is incomplete (when emitted
// as a placeholder with length == 0) or *why* its payload was clamped
// (when emitted on a real, captured item). Kept in sync with
// data_item_reason_t in pkg/dyninst/ebpf/framing.h.
type DataItemReason uint8

const (
	// DataItemReasonNone is the default for items the SM emits without
	// applying any limit.
	DataItemReasonNone DataItemReason = 0

	// Placeholder reasons (item has DataItemFailedReadMask set and
	// Length == 0). These describe *why* a chased pointee was omitted
	// from the capture entirely.

	// DataItemReasonTooManyPointersInFlight: the in-flight pointers queue
	// was full when this pointer needed to be followed.
	DataItemReasonTooManyPointersInFlight DataItemReason = 1
	// DataItemReasonTooManyUniquePointers: the per-event dedup table of
	// seen pointer addresses was full.
	DataItemReasonTooManyUniquePointers DataItemReason = 2
	// DataItemReasonTooManySlicesCaptured: the per-event captured-slices
	// table was full.
	DataItemReasonTooManySlicesCaptured DataItemReason = 3
	// DataItemReasonCaptureNestingTooDeep: the stack machine's recursion
	// stack was full while capturing this value.
	DataItemReasonCaptureNestingTooDeep DataItemReason = 4

	// Real-item reasons (item carries actual bytes; reason describes
	// how the payload was clamped).

	// DataItemReasonValueTooLarge: serialize length was clamped to
	// MaxDataItemSize (8 KiB).
	DataItemReasonValueTooLarge DataItemReason = 5
	// DataItemReasonStringSize: the configured string MaxLength clamped
	// the payload.
	DataItemReasonStringSize DataItemReason = 6
	// DataItemReasonCollectionSize: the configured MaxCollectionSize
	// clamped the element count.
	DataItemReasonCollectionSize DataItemReason = 7

	// 8..14 reserved.

	// DataItemReasonExtended is reserved for a future side-channel
	// reason-table mechanism. Not produced by current eBPF code.
	DataItemReasonExtended DataItemReason = 15
)

const (
	// ContinuationFlagMore indicates that more fragments follow this one.
	ContinuationFlagMore = uint8(1)
)

// FragmentedEvent provides access to one or more event fragments that together
// represent a single logical event. The first fragment carries the event
// header, stack trace, and root data item. Subsequent fragments carry only
// additional pointer-chased data items.
type FragmentedEvent interface {
	Fragments() iter.Seq[Event]
}

// SingleEvent wraps a single Event as a FragmentedEvent. It yields itself once.
type SingleEvent Event

// Fragments implements FragmentedEvent.
func (e SingleEvent) Fragments() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		yield(Event(e))
	}
}

// DataItem represents a single data item in an event.
type DataItem struct {
	header *DataItemHeader
	data   []byte
}

// IsFailedRead returns true if the data item was marked as a failed read.
func (d *DataItem) IsFailedRead() bool {
	return d.header.Type&DataItemFailedReadMask != 0
}

// Type returns the type ID of the data item, with the failed-read and
// reason bits stripped.
func (d *DataItem) Type() uint32 {
	return d.header.Type & DataItemTypeMask
}

// Reason returns the DataItemReason packed into bits 27..30 of the type
// field. Returns DataItemReasonNone when no reason was set.
func (d *DataItem) Reason() DataItemReason {
	return DataItemReason((d.header.Type & DataItemReasonMask) >> DataItemReasonShift)
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

// IsContinuation returns true if this event is part of a multi-fragment
// continuation (either as the first fragment with more to follow, or as a
// subsequent fragment).
func (h *EventHeader) IsContinuation() bool {
	return h.Continuation_seq > 0 || h.Continuation_flags&ContinuationFlagMore != 0
}

// HasMoreFragments returns true if more continuation fragments are expected
// after this one.
func (h *EventHeader) HasMoreFragments() bool {
	return h.Continuation_flags&ContinuationFlagMore != 0
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
