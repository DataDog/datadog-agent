// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2/hpack"
)

const (
	maxPathLen          = 30
	compressedPathLen   = 21
	decompressedPathLen = 28
)

func TestHTTP2Path(t *testing.T) {
	t.Run("validate http2 path backslash", func(t *testing.T) {
		// create a buffer to store the encoded data
		paths := []string{"/hello.HelloService/SayHello", "hello.HelloService/SayHello"}
		results := []bool{true, false}
		pathsResults := []string{"/hello.HelloService/SayHello", ""}

		for index, currentString := range paths {
			var buf []byte
			var arr [maxPathLen]uint8
			buf = hpack.AppendHuffmanString(buf, currentString)
			copy(arr[:], buf[:30])

			request := &EbpfTx{
				Request_path: arr,
				Path_size:    compressedPathLen,
			}

			outBuf := make([]byte, decompressedPathLen)

			path, ok := request.Path(outBuf)
			assert.Equal(t, results[index], ok)
			assert.Equal(t, pathsResults[index], string(path))
		}

	})

	t.Run("empty path", func(t *testing.T) {
		request := &EbpfTx{
			Request_path: [maxPathLen]uint8{},
			Path_size:    compressedPathLen,
		}
		outBuf := make([]byte, decompressedPathLen)

		path, ok := request.Path(outBuf)
		assert.Equal(t, ok, false)
		assert.Nil(t, path)
	})
}
