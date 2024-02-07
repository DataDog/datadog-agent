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

				request := &EbpfTx{
					Stream: http2Stream{
						Is_huffman_encoded: huffmanEnabled,
						Request_path:       arr,
						Path_size:          uint8(len(buf)),
					},
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
		Stream http2Stream
		want   http.Method
	}{
		{
			name: "Sanity method test Method",
			Stream: http2Stream{
				Request_method: http2requestMethod{
					Raw_buffer:         [7]uint8{0x50, 0x55, 0x54},
					Is_huffman_encoded: false,
					Static_table_entry: 0,
					Length:             3,
					Finalized:          false,
				},
			},
			want: http.MethodPut,
		},
		{
			name: "Test method length is bigger than raw buffer size",
			Stream: http2Stream{
				Request_method: http2requestMethod{
					Raw_buffer:         [7]uint8{1, 2},
					Is_huffman_encoded: false,
					Static_table_entry: 0,
					Length:             8,
					Finalized:          false,
				},
			},
			want: http.MethodUnknown,
		},
		{
			name: "Test method length is zero",
			Stream: http2Stream{
				Request_method: http2requestMethod{
					Raw_buffer:         [7]uint8{1, 2},
					Is_huffman_encoded: true,
					Static_table_entry: 0,
					Length:             0,
					Finalized:          false,
				},
			},
			want: http.MethodUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &EbpfTx{
				Stream: tt.Stream,
			}
			assert.Equalf(t, tt.want, tx.Method(), "Method()")
		})
	}
}
