// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// We rely on the fact that the header and data item header are aligned to 8 bytes.
// This test ensures that this is the case.
func TestHeaderAlignment(t *testing.T) {
	require.Equal(t, 0, int(unsafe.Sizeof(EventHeader{}))%8)
	require.Equal(t, 0, int(unsafe.Sizeof(DataItemHeader{}))%8)
}

var (
	fullStack = []uint64{0x1, 0x2}
	fullItems = []DataItem{
		{
			header: &DataItemHeader{Type: 1, Length: 4, Address: 0x100},
			data:   []byte{1, 2, 3, 4},
		},
		{
			header: &DataItemHeader{Type: 2, Length: 8, Address: 0x200},
			data:   []byte{5, 6, 7, 8, 9, 10, 11, 12},
		},
	}
	fullItemsDataLen = func() uint32 {
		l := uint32(0)
		for _, item := range fullItems {
			l += uint32(
				dataItemHeaderSize +
					nextMultipleOf8(int(item.header.Length)),
			)
		}
		return l
	}()
	fullHeader = EventHeader{
		Prog_id:        1,
		Stack_byte_len: 16,
		Stack_hash:     12345,
		Ktime_ns:       1000,
		Data_byte_len:  uint32(int(eventHeaderSize) + 16 + int(fullItemsDataLen)),
	}
	validEvent = Event(buildEvent(nil, &fullHeader, fullStack, fullItems))
)

func BenchmarkFirstDataItemHeader(b *testing.B) {
	var v *DataItemHeader
	for b.Loop() {
		v, _ = validEvent.FirstDataItemHeader()
	}
	require.NotNil(b, v)
}
func BenchmarkHeader(b *testing.B) {
	var v *EventHeader
	for i := 0; i < b.N; i++ {
		v, _ = validEvent.Header()
	}
	require.NotNil(b, v)
	require.Equal(b, &fullHeader, v)
}
func BenchmarkStackPCs(b *testing.B) {
	var v []uint64
	for b.Loop() {
		v, _ = validEvent.StackPCs()
	}
	require.NotNil(b, v)
	require.Equal(b, fullStack, v)
}

func BenchmarkDataItems(b *testing.B) {
	items := []DataItem{}
	for b.Loop() {
		items = items[:0]
		for item := range validEvent.DataItems() {
			items = append(items, item)
		}
	}
	require.EqualValues(b, fullItems, items)
}

func TestEventIterator(t *testing.T) {
	tests := []struct {
		name               string
		event              Event
		expectedHeader     *EventHeader
		expectHeaderErr    string
		expectedStack      []uint64
		expectStackErr     string
		expectedDataItems  []DataItem
		expectDataItemsErr []string
	}{
		{
			name:              "valid event",
			event:             buildEvent(nil, &fullHeader, fullStack, fullItems),
			expectedHeader:    &fullHeader,
			expectedStack:     fullStack,
			expectedDataItems: fullItems,
		},
		{
			name:               "event too short for header",
			event:              []byte{1, 2, 3},
			expectHeaderErr:    "not enough bytes to read event header",
			expectStackErr:     "not enough bytes to read event header",
			expectDataItemsErr: []string{"not enough bytes to read event header"},
		},
		{
			name: "event length mismatch",
			event: func() []byte {
				header := fullHeader
				header.Data_byte_len = uint32(eventHeaderSize + 100)
				return buildEvent(nil, &header, fullStack, fullItems)
			}(),
			expectHeaderErr:    "event length mismatch",
			expectStackErr:     "event length mismatch",
			expectDataItemsErr: []string{"event length mismatch"},
		},
		{
			name: "event too short for stack",
			event: func() []byte {
				header := EventHeader{
					Stack_byte_len: 16,
					Data_byte_len:  uint32(eventHeaderSize),
				}
				b := buildEvent(nil, &header, nil, nil)
				return b[:eventHeaderSize] // trim to not include padding
			}(),
			expectedHeader: &EventHeader{
				Stack_byte_len: 16,
				Data_byte_len:  uint32(eventHeaderSize),
			},
			expectStackErr:     "not enough bytes to read stack trace",
			expectDataItemsErr: []string{"not enough bytes to read stack trace"},
		},
		{
			name: "stack length not multiple of 8",
			event: func() []byte {
				header := EventHeader{
					Stack_byte_len: 15,
					Data_byte_len:  uint32(eventHeaderSize + 15),
				}
				buf := buildEvent(nil, &header, []uint64{1, 2}, nil)
				return buf[:header.Data_byte_len]
			}(),
			expectedHeader: &EventHeader{
				Stack_byte_len: 15,
				Data_byte_len:  uint32(eventHeaderSize + 15),
			},
			expectStackErr:     "stack trace length is not a multiple of 8",
			expectDataItemsErr: []string{"stack trace length is not a multiple of 8"},
		},
		{
			name: "event with no stack",
			event: func() []byte {
				header := fullHeader
				header.Stack_byte_len = 0
				header.Stack_hash = 0
				header.Data_byte_len = uint32(eventHeaderSize) + fullItemsDataLen
				return buildEvent(nil, &header, nil, fullItems)
			}(),
			expectedHeader: func() *EventHeader {
				header := fullHeader
				header.Stack_byte_len = 0
				header.Stack_hash = 0
				header.Data_byte_len = uint32(eventHeaderSize) + fullItemsDataLen
				return &header
			}(),
			expectedStack:     nil,
			expectedDataItems: fullItems,
		},
		{
			name: "data item header truncated",
			event: func() []byte {
				header := fullHeader
				header.Data_byte_len = uint32(eventHeaderSize) +
					uint32(header.Stack_byte_len) +
					1
				event := buildEvent(nil, &header, fullStack, fullItems)
				return event[:header.Data_byte_len]
			}(),
			expectedHeader: func() *EventHeader {
				header := fullHeader
				header.Data_byte_len = uint32(eventHeaderSize) +
					uint32(header.Stack_byte_len) +
					1
				return &header
			}(),
			expectedStack:      fullStack,
			expectDataItemsErr: []string{"not enough bytes to read data item header:"},
		},
		{
			name: "one valid data item, one truncated data item",
			event: func() []byte {
				header := fullHeader
				totalSize := eventHeaderSize +
					int(fullHeader.Stack_byte_len) +
					dataItemHeaderSize +
					nextMultipleOf8(int(fullItems[0].header.Length)) +
					dataItemHeaderSize +
					4
				header.Data_byte_len = uint32(totalSize)
				buf := buildEvent(nil, &header, fullStack, fullItems)
				return buf[:totalSize]
			}(),
			expectedHeader: func() *EventHeader {
				header := fullHeader
				totalSize := eventHeaderSize +
					int(fullHeader.Stack_byte_len) +
					dataItemHeaderSize +
					nextMultipleOf8(int(fullItems[0].header.Length)) +
					dataItemHeaderSize +
					4
				header.Data_byte_len = uint32(totalSize)
				return &header
			}(),
			expectedStack:     fullStack,
			expectedDataItems: []DataItem{fullItems[0]},
			expectDataItemsErr: []string{"", fmt.Sprintf(
				`not enough bytes to read data item \(8 bytes\): %d < %d`,
				eventHeaderSize+int(fullHeader.Stack_byte_len)+
					2*dataItemHeaderSize+nextMultipleOf8(int(fullItems[0].header.Length))+4,
				eventHeaderSize+int(fullHeader.Stack_byte_len)+
					2*dataItemHeaderSize+nextMultipleOf8(int(fullItems[0].header.Length))+
					int(fullItems[1].header.Length),
			)},
		},
		{
			name: "one valid data item, one truncated data item header",
			event: func() []byte {
				header := fullHeader
				totalSize := eventHeaderSize +
					int(fullHeader.Stack_byte_len) +
					dataItemHeaderSize +
					nextMultipleOf8(int(fullItems[0].header.Length)) +
					4
				header.Data_byte_len = uint32(totalSize)
				buf := buildEvent(nil, &header, fullStack, fullItems)
				return buf[:totalSize]
			}(),
			expectedHeader: func() *EventHeader {
				header := fullHeader
				totalSize := eventHeaderSize +
					int(fullHeader.Stack_byte_len) +
					dataItemHeaderSize +
					nextMultipleOf8(int(fullItems[0].header.Length)) +
					4
				header.Data_byte_len = uint32(totalSize)
				return &header
			}(),
			expectedStack:      fullStack,
			expectedDataItems:  []DataItem{fullItems[0]},
			expectDataItemsErr: []string{"", "not enough bytes to read data item header:"},
		},
		{
			name: "unaligned data",
			event: func() []byte {
				buf := make([]byte, 1024)
				buf = buf[1:1]
				return buildEvent(buf, &fullHeader, fullStack, fullItems)
			}(),
			expectHeaderErr:    "event data is not aligned",
			expectStackErr:     "event data is not aligned",
			expectDataItemsErr: []string{"event data is not aligned"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, err := tt.event.Header()
			if tt.expectHeaderErr != "" {
				require.Error(t, err)
				require.Regexp(t, tt.expectHeaderErr, err)
			} else {
				require.NoError(t, err)
				if tt.expectedHeader != nil {
					require.Equal(t, tt.expectedHeader, header)
				}
			}

			stack, err := tt.event.StackPCs()
			if tt.expectStackErr != "" {
				require.Regexp(t, tt.expectStackErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedStack, stack)
			}

			var items []DataItem
			i := 0
			for item, err := range tt.event.DataItems() {
				if len(tt.expectDataItemsErr) > i {
					pattern := tt.expectDataItemsErr[i]
					if pattern != "" {
						require.Regexp(t, pattern, err)
					} else {
						require.NoError(t, err)
						data, ok := item.Data()
						require.True(t, ok)
						header := *item.Header()
						items = append(items, DataItem{
							header: &header,
							data:   data,
						})
					}
				} else {
					require.NoError(
						t, err,
						"got unexpected error from DataItems iterator on item %d",
						i,
					)
					data, ok := item.Data()
					require.True(t, ok)
					header := *item.Header()
					items = append(items, DataItem{
						header: &header,
						data:   data,
					})
				}
				i++
			}

			if tt.expectDataItemsErr != nil {
				require.Equal(
					t,
					len(tt.expectDataItemsErr),
					i,
					"iterator yielded a different number of items/errors than expected",
				)
			}
			require.Equal(t, len(tt.expectedDataItems), len(items))
			for i := range tt.expectedDataItems {
				require.Equal(
					t,
					tt.expectedDataItems[i].header,
					items[i].header,
					"item %d header",
					i,
				)
				require.Equal(
					t,
					tt.expectedDataItems[i].data,
					items[i].data,
					"item %d data",
					i,
				)
			}
			// Exercise early return.
			for range tt.event.DataItems() {
				break
			}
		})
	}
}

func TestContinuationFlags(t *testing.T) {
	tests := []struct {
		name             string
		seq              uint16
		flags            uint8
		isContinuation   bool
		hasMoreFragments bool
	}{
		{
			name:             "legacy single event",
			seq:              0,
			flags:            0,
			isContinuation:   false,
			hasMoreFragments: false,
		},
		{
			name:             "first fragment with more",
			seq:              0,
			flags:            ContinuationFlagMore,
			isContinuation:   true,
			hasMoreFragments: true,
		},
		{
			name:             "middle fragment",
			seq:              1,
			flags:            ContinuationFlagMore,
			isContinuation:   true,
			hasMoreFragments: true,
		},
		{
			name:             "final fragment",
			seq:              2,
			flags:            0,
			isContinuation:   true,
			hasMoreFragments: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &EventHeader{
				Continuation_seq:   tt.seq,
				Continuation_flags: tt.flags,
			}
			require.Equal(t, tt.isContinuation, h.IsContinuation())
			require.Equal(t, tt.hasMoreFragments, h.HasMoreFragments())
		})
	}
}

func TestMultiFragmentReassembly(t *testing.T) {
	// Build a 3-fragment event and verify that concatenating the data item
	// portions of fragments 1 and 2 onto fragment 0 produces a valid event
	// that the DataItems iterator can walk.

	rootItem := DataItem{
		header: &DataItemHeader{Type: 1, Length: 8, Address: 0x100},
		data:   []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	itemA := DataItem{
		header: &DataItemHeader{Type: 2, Length: 4, Address: 0x200},
		data:   []byte{10, 11, 12, 13},
	}
	itemB := DataItem{
		header: &DataItemHeader{Type: 3, Length: 8, Address: 0x300},
		data:   []byte{20, 21, 22, 23, 24, 25, 26, 27},
	}
	itemC := DataItem{
		header: &DataItemHeader{Type: 4, Length: 4, Address: 0x400},
		data:   []byte{30, 31, 32, 33},
	}

	stack := []uint64{0xdead, 0xbeef}
	allItems := []DataItem{rootItem, itemA, itemB, itemC}

	// Fragment 0: header + stack + rootItem + itemA
	frag0Header := EventHeader{
		Prog_id:            1,
		Goid:               42,
		Stack_byte_depth:   100,
		Probe_id:           7,
		Stack_byte_len:     16,
		Ktime_ns:           5000,
		Continuation_seq:   0,
		Continuation_flags: ContinuationFlagMore,
	}
	frag0Items := []DataItem{rootItem, itemA}
	frag0 := buildEvent(nil, &frag0Header, stack, frag0Items)
	frag0Header.Data_byte_len = uint32(len(frag0))
	*(*EventHeader)(unsafe.Pointer(&frag0[0])) = frag0Header

	// Fragment 1: header + itemB (no stack)
	frag1Header := EventHeader{
		Prog_id:            1,
		Goid:               42,
		Stack_byte_depth:   100,
		Probe_id:           7,
		Stack_byte_len:     0,
		Ktime_ns:           5000,
		Continuation_seq:   1,
		Continuation_flags: ContinuationFlagMore,
	}
	frag1 := buildEvent(nil, &frag1Header, nil, []DataItem{itemB})
	frag1Header.Data_byte_len = uint32(len(frag1))
	*(*EventHeader)(unsafe.Pointer(&frag1[0])) = frag1Header

	// Fragment 2: header + itemC (final)
	frag2Header := EventHeader{
		Prog_id:            1,
		Goid:               42,
		Stack_byte_depth:   100,
		Probe_id:           7,
		Stack_byte_len:     0,
		Ktime_ns:           5000,
		Continuation_seq:   2,
		Continuation_flags: 0, // final
	}
	frag2 := buildEvent(nil, &frag2Header, nil, []DataItem{itemC})
	frag2Header.Data_byte_len = uint32(len(frag2))
	*(*EventHeader)(unsafe.Pointer(&frag2[0])) = frag2Header

	// Simulate reassembly: base = frag0, append data-item bytes from frag1
	// and frag2 (everything after the event header).
	reassembled := make([]byte, len(frag0))
	copy(reassembled, frag0)
	reassembled = append(reassembled, frag1[eventHeaderSize:]...)
	reassembled = append(reassembled, frag2[eventHeaderSize:]...)

	// Fix up the header.
	rh := (*EventHeader)(unsafe.Pointer(&reassembled[0]))
	rh.Data_byte_len = uint32(len(reassembled))
	rh.Continuation_seq = 0
	rh.Continuation_flags = 0

	ev := Event(reassembled)
	header, err := ev.Header()
	require.NoError(t, err)
	require.Equal(t, uint32(len(reassembled)), header.Data_byte_len)

	// Verify stack
	pcs, err := ev.StackPCs()
	require.NoError(t, err)
	require.Equal(t, stack, pcs)

	// Verify all data items are present
	var items []DataItem
	for item, err := range ev.DataItems() {
		require.NoError(t, err)
		data, ok := item.Data()
		require.True(t, ok)
		hdr := *item.Header()
		items = append(items, DataItem{header: &hdr, data: data})
	}
	require.Len(t, items, len(allItems))
	for i := range allItems {
		require.Equal(t, allItems[i].header, items[i].header, "item %d header", i)
		require.Equal(t, allItems[i].data, items[i].data, "item %d data", i)
	}
}

func alignTo8(b []byte) []byte {
	for i, n := 0, nextMultipleOf8(len(b))-len(b); i < n; i++ {
		b = append(b, 0)
	}
	return b
}

func buildEvent(
	b []byte,
	header *EventHeader,
	stack []uint64,
	items []DataItem,
) []byte {
	b = append(b[:0], make([]byte, eventHeaderSize)...)
	*(*EventHeader)(unsafe.Pointer(&b[0])) = *header

	if len(stack) > 0 {
		stackBytes := unsafe.Slice((*byte)(unsafe.Pointer(&stack[0])), len(stack)*8)
		b = append(b, stackBytes...)
	}
	b = alignTo8(b)

	for _, item := range items {
		itemHeaderStart := len(b)
		b = append(b, make([]byte, dataItemHeaderSize)...)
		*(*DataItemHeader)(unsafe.Pointer(&b[itemHeaderStart])) = *item.header

		b = append(b, item.data...)
		b = alignTo8(b)
	}

	return b
}
