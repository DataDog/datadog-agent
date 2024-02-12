// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

func TestHTTP2Path(t *testing.T) {
	tests := []struct {
		name        string
		rawPath     string
		expectedErr bool
	}{
		{
			name:    "Short path",
			rawPath: "/hello.HelloService/SayHello",
		},
		{
			name:    "Long path",
			rawPath: "/resourcespb.ResourceTagging/GetResourceTags",
		},
		{
			name:        "Path does not start with /",
			rawPath:     "hello.HelloService/SayHello",
			expectedErr: true,
		},
		{
			name:        "Empty path",
			rawPath:     "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		for _, huffmanEnabled := range []bool{false, true} {
			testNameSuffix := fmt.Sprintf("huffman-enabled=%v", huffmanEnabled)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				var buf []byte
				var arr [maxHTTP2Path]uint8
				if huffmanEnabled {
					buf = hpack.AppendHuffmanString(buf, tt.rawPath)
				} else {
					buf = append(buf, tt.rawPath...)
				}
				copy(arr[:], buf)

				dynamicTable := NewDynamicTable(10)
				dynamicTable.handleNewDynamicTableEntry(&http2DynamicTableValue{
					Key: HTTP2DynamicTableIndex{
						Index: 1,
					},
					Is_huffman_encoded: huffmanEnabled,
					String_len:         uint8(len(buf)),
					Buf:                arr,
				})
				request := &ebpfTXWrapper{
					EbpfTx: &EbpfTx{
						Stream: http2Stream{
							Path: http2InterestingValue{
								Index: http2staticTableMaxEntry + 1,
							},
						},
					},
					dynamicTable: dynamicTable,
				}

				outBuf := make([]byte, 200)

				path, ok := request.Path(outBuf)
				if tt.expectedErr {
					assert.False(t, ok)
					return
				}
				assert.True(t, ok)
				assert.Equal(t, tt.rawPath, string(path))
			})
		}
	}
}

func TestHTTP2Method(t *testing.T) {
	tests := []struct {
		name   string
		buffer []byte
		length uint8
		want   http.Method
	}{
		{
			name:   "Sanity method test",
			buffer: []byte{0x50, 0x55, 0x54},
			length: 3,
			want:   http.MethodPut,
		},
		{
			name:   "Test method length is bigger than raw buffer size",
			buffer: []byte{1, 2},
			length: 8,
			want:   http.MethodUnknown,
		},
		{
			name:   "Test method length is zero",
			buffer: []byte{1, 2},
			length: 0,
			want:   http.MethodUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var arr [maxHTTP2Path]uint8
			copy(arr[:], tt.buffer)

			dynamicTable := NewDynamicTable(1)
			dynamicTable.handleNewDynamicTableEntry(&http2DynamicTableValue{
				Key: HTTP2DynamicTableIndex{
					Index: http2staticTableMaxEntry + 1,
				},
				String_len: tt.length,
				Buf:        arr,
			})
			tx := &ebpfTXWrapper{
				EbpfTx: &EbpfTx{
					Stream: http2Stream{
						Request_method: http2InterestingValue{
							Index: http2staticTableMaxEntry + 1,
						},
					},
				},
				dynamicTable: dynamicTable,
			}
			assert.Equalf(t, tt.want, tx.Method(), "Method()")
		})
	}
}
