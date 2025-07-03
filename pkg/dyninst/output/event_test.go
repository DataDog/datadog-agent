// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

type testDataItem struct {
	header DataItemHeader
	data   []byte
}

// We rely on the fact that the header and data item header are aligned to 8 bytes.
// This test ensures that this is the case.
func TestHeaderAlignment(t *testing.T) {
	require.Equal(t, 0, int(unsafe.Sizeof(EventHeader{}))%8)
	require.Equal(t, 0, int(unsafe.Sizeof(DataItemHeader{}))%8)
}

func TestEventIterator(t *testing.T) {
	fullHeader := EventHeader{
		Prog_id:        1,
		Stack_byte_len: 16,
		Stack_hash:     12345,
		Ktime_ns:       1000,
	}
	fullStack := []uint64{0x1, 0x2}
	fullItems := []testDataItem{
		{
			header: DataItemHeader{Type: 1, Length: 4, Address: 0x100},
			data:   []byte{1, 2, 3, 4},
		},
		{
			header: DataItemHeader{Type: 2, Length: 8, Address: 0x200},
			data:   []byte{5, 6, 7, 8, 9, 10, 11, 12},
		},
	}
	var fullItemsDataLen uint32
	for _, item := range fullItems {
		fullItemsDataLen += uint32(
			dataItemHeaderSize +
				nextMultipleOf8(int(item.header.Length)),
		)
	}
	fullHeader.Data_byte_len = uint32(
		eventHeaderSize +
			nextMultipleOf8(int(fullHeader.Stack_byte_len)) +
			int(fullItemsDataLen),
	)

	tests := []struct {
		name               string
		event              Event
		expectedHeader     *EventHeader
		expectHeaderErr    string
		expectedStack      []uint64
		expectStackErr     string
		expectedDataItems  []testDataItem
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
			expectedStack:      fullStack,
			expectedDataItems:  []testDataItem{fullItems[0]},
			expectDataItemsErr: []string{"", "not enough bytes to read data item:"},
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
			expectedDataItems:  []testDataItem{fullItems[0]},
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

			var items []testDataItem
			i := 0
			for item, err := range tt.event.DataItems() {
				if len(tt.expectDataItemsErr) > i {
					pattern := tt.expectDataItemsErr[i]
					if pattern != "" {
						require.Regexp(t, pattern, err)
					} else {
						require.NoError(t, err)
						items = append(
							items,
							testDataItem{
								header: *item.Header(),
								data:   item.Data(),
							},
						)
					}
				} else {
					require.NoError(
						t, err,
						"got unexpected error from DataItems iterator on item %d",
						i,
					)
					items = append(
						items,
						testDataItem{header: *item.Header(), data: item.Data()},
					)
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
	items []testDataItem,
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
		*(*DataItemHeader)(unsafe.Pointer(&b[itemHeaderStart])) = item.header

		b = append(b, item.data...)
		b = alignTo8(b)
	}

	return b
}
