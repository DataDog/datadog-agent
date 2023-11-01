// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	//"runtime"
	//"strings"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/stretchr/testify/assert"
)

func requestFragment(fragment []byte, buffsize int) []byte {
	b := make([]byte, buffsize)

	copy(b[:], fragment)
	return b
}

func TestPath(t *testing.T) {
	cfg := config.New()
	BufferSize := int(cfg.HTTPMaxRequestFragment)
	tx := WinHttpTransaction{
		RequestFragment: requestFragment([]byte("GET /foo/bar?var1=value HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"), BufferSize),
	}
	b := make([]byte, cfg.HTTPMaxRequestFragment)
	path, fullPath := tx.Path(b)
	assert.Equal(t, "/foo/bar", string(path))
	assert.True(t, fullPath)
}

func TestMaximumLengthPath(t *testing.T) {
	cfg := config.New()
	BufferSize := int(cfg.HTTPMaxRequestFragment)
	rep := strings.Repeat("a", BufferSize-6)
	str := "GET /" + rep
	str += "bc"
	tx := WinHttpTransaction{
		RequestFragment: requestFragment([]byte(str), BufferSize),
	}
	b := make([]byte, BufferSize)
	path, fullPath := tx.Path(b)
	expected := "/" + rep
	expected = expected + "b"
	assert.Equal(t, expected, string(path))
	assert.False(t, fullPath)

}

func TestFullPath(t *testing.T) {
	cfg := config.New()
	BufferSize := int(cfg.HTTPMaxRequestFragment)

	prefix := "GET /"
	rep := strings.Repeat("a", BufferSize-len(prefix)-1)
	str := prefix + rep + " "
	tx := WinHttpTransaction{
		RequestFragment: requestFragment([]byte(str), BufferSize),
	}
	b := make([]byte, BufferSize)
	path, fullPath := tx.Path(b)
	expected := "/" + rep
	assert.Equal(t, expected, string(path))
	assert.True(t, fullPath)
}

func TestPathHandlesNullTerminator(t *testing.T) {
	cfg := config.New()
	BufferSize := int(cfg.HTTPMaxRequestFragment)

	tx := WinHttpTransaction{
		// This probably isn't a valid HTTP request
		// (since it's missing a version before the end),
		// but if the null byte isn't handled
		// then the path becomes "/foo/\x00bar"

		RequestFragment: requestFragment([]byte("GET /foo/\x00bar?var1=value HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"), BufferSize),
	}
	b := make([]byte, BufferSize)
	path, fullPath := tx.Path(b)
	assert.Equal(t, "/foo/", string(path))
	assert.False(t, fullPath)
}

func TestLatency(t *testing.T) {
	tx := WinHttpTransaction{
		Txn: driver.HttpTransactionType{
			ResponseLastSeen: 2e6,
			RequestStarted:   1e6,
		},
	}

	// quantization brings it down
	assert.Equal(t, 999424.0, tx.RequestLatency())
}
