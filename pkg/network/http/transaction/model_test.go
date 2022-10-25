// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package transaction

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath(t *testing.T) {
	tx := EbpfHttpTx{
		Request_fragment: RequestFragment(
			[]byte("GET /foo/bar?var1=value HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"),
		),
	}

	b := make([]byte, HTTPBufferSize)
	path, fullPath := tx.Path(b)
	assert.Equal(t, "/foo/bar", string(path))
	assert.True(t, fullPath)
}

func TestMaximumLengthPath(t *testing.T) {
	rep := strings.Repeat("a", HTTPBufferSize-6)
	str := "GET /" + rep
	str += "bc"
	tx := EbpfHttpTx{
		Request_fragment: RequestFragment(
			[]byte(str),
		),
	}
	b := make([]byte, HTTPBufferSize)
	path, fullPath := tx.Path(b)
	expected := "/" + rep
	expected = expected + "b"
	assert.Equal(t, expected, string(path))
	assert.False(t, fullPath)
}

func TestPathHandlesNullTerminator(t *testing.T) {
	tx := EbpfHttpTx{
		Request_fragment: RequestFragment(
			// This probably isn't a valid HTTP request
			// (since it's missing a version before the end),
			// but if the null byte isn't handled
			// then the path becomes "/foo/\x00bar"
			[]byte("GET /foo/\x00bar?var1=value HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"),
		),
	}

	b := make([]byte, HTTPBufferSize)
	path, fullPath := tx.Path(b)
	assert.Equal(t, "/foo/", string(path))
	assert.False(t, fullPath)
}

func TestLatency(t *testing.T) {
	tx := EbpfHttpTx{
		Response_last_seen: 2e6,
		Request_started:    1e6,
	}
	// quantization brings it down
	assert.Equal(t, 999424.0, tx.RequestLatency())
}

func BenchmarkPath(b *testing.B) {
	tx := EbpfHttpTx{
		Request_fragment: RequestFragment(
			[]byte("GET /foo/bar?var1=value HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"),
		),
	}

	b.ReportAllocs()
	b.ResetTimer()
	buf := make([]byte, HTTPBufferSize)
	for i := 0; i < b.N; i++ {
		_, _ = tx.Path(buf)
	}
	runtime.KeepAlive(buf)
}
