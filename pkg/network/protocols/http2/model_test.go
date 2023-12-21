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
