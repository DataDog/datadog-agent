// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	http2MaxPathLen     = 30
	compressedPathLen   = 21
	decompressedPathLen = 28
)

func TestOrphanEntries(t *testing.T) {
	t.Run("orphan entries can be joined even after flushing", func(t *testing.T) {
		now := time.Now()
		tel := NewTelemetry()
		buffer := newIncompleteBuffer(config.New(), tel)
		request := &EbpfHttpTx{
			Request_fragment: requestFragment([]byte("GET /foo/bar")),
			Request_started:  uint64(now.UnixNano()),
		}
		request.Tup.Sport = 60000

		buffer.Add(request)
		now = now.Add(5 * time.Second)
		complete := buffer.Flush(now)
		assert.Len(t, complete, 0)

		response := &EbpfHttpTx{
			Response_status_code: 200,
			Response_last_seen:   uint64(now.UnixNano()),
		}
		response.Tup.Sport = 60000
		buffer.Add(response)
		complete = buffer.Flush(now)
		require.Len(t, complete, 1)

		completeTX := complete[0]
		path, _ := completeTX.Path(make([]byte, 256))
		assert.Equal(t, "/foo/bar", string(path))
		assert.Equal(t, uint16(200), completeTX.StatusCode())
	})

	t.Run("orphan entries are not kept indefinitely", func(t *testing.T) {
		tel := NewTelemetry()
		buffer := newIncompleteBuffer(config.New(), tel)
		now := time.Now()
		buffer.minAgeNano = (30 * time.Second).Nanoseconds()
		request := &EbpfHttpTx{
			Request_fragment: requestFragment([]byte("GET /foo/bar")),
			Request_started:  uint64(now.UnixNano()),
		}
		buffer.Add(request)
		_ = buffer.Flush(now)

		assert.True(t, len(buffer.data) > 0)
		now = now.Add(35 * time.Second)
		_ = buffer.Flush(now)
		assert.True(t, len(buffer.data) == 0)
	})
}

func TestHTTP2Path(t *testing.T) {
	t.Run("validate http2 path backslash", func(t *testing.T) {
		// create a buffer to store the encoded data
		paths := []string{"/hello.HelloService/SayHello", "hello.HelloService/SayHello"}
		results := []bool{true, false}
		pathsResults := []string{"/hello.HelloService/SayHello", ""}

		for index, currentString := range paths {
			var buf []byte
			var arr [http2MaxPathLen]uint8
			buf = hpack.AppendHuffmanString(buf, currentString)
			copy(arr[:], buf[:30])

			request := &EbpfHttp2Tx{
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
		request := &EbpfHttp2Tx{
			Request_path: [http2MaxPathLen]uint8{},
			Path_size:    compressedPathLen,
		}
		outBuf := make([]byte, decompressedPathLen)

		path, ok := request.Path(outBuf)
		assert.Equal(t, ok, false)
		assert.Nil(t, path)
	})
}
