// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

func requestFragment(fragment []byte, buffsize int) []byte {
	b := make([]byte, buffsize)

	copy(b[:], fragment)
	return b
}

func getBufferSize() int {
	cfg := config.New()
	return int(cfg.HTTPMaxRequestFragment)
}

func makeTxnFromRequestString(s string) WinHttpTransaction {
	return WinHttpTransaction{
		RequestFragment: requestFragment([]byte(s), getBufferSize()),
	}
}

func makeTxnFromLatency(lastSeen, started uint64) WinHttpTransaction {
	return WinHttpTransaction{
		Txn: driver.HttpTransactionType{
			ResponseLastSeen: lastSeen,
			RequestStarted:   started,
		},
	}
}

func TestParseMethodFromFragment(t *testing.T) {
	cases := map[string]Method{
		"GET / HTTP/1.1\r\n":          MethodGet,
		"POST /api/data HTTP/1.1\r\n": MethodPost,
		"PUT /x HTTP/1.1\r\n":         MethodPut,
		"DELETE /x HTTP/1.1\r\n":      MethodDelete,
		"HEAD / HTTP/1.1\r\n":         MethodHead,
		"OPTIONS * HTTP/1.1\r\n":      MethodOptions,
		"PATCH /x HTTP/1.1\r\n":       MethodPatch,
		"NOTAMETHOD /x HTTP/1.1\r\n":  MethodUnknown,
		"":                            MethodUnknown,
	}
	for frag, want := range cases {
		assert.Equalf(t, want, parseMethodFromFragment([]byte(frag)), "fragment %q", frag)
	}
}

func TestParseStatusFromFragment(t *testing.T) {
	cases := map[string]uint16{
		"HTTP/1.1 200 OK\r\n":                    200,
		"HTTP/1.1 404 Not Found\r\n":             404,
		"HTTP/1.1 500 Internal Server Error\r\n": 500,
		"HTTP/2 301 Moved\r\n":                   301,
		"HTTP/1.1 99 Invalid\r\n":                0,
		"HTTP/1.1 600 Invalid\r\n":               0,
		"HTTP/1.1 ABC\r\n":                       0,
		"garbage":                                0,
		"":                                       0,
	}
	for frag, want := range cases {
		assert.Equalf(t, want, parseStatusFromFragment([]byte(frag)), "fragment %q", frag)
	}
}

// TestModeSwitch verifies Method()/StatusCode() honor the ParseInAgent toggle: in agent-parse
// mode they read the raw fragments; in legacy mode they trust the driver-populated fields.
func TestModeSwitch(t *testing.T) {
	agentMode := WinHttpTransaction{
		ParseInAgent:     true,
		RequestFragment:  []byte("POST /v1/x HTTP/1.1\r\n"),
		ResponseFragment: []byte("HTTP/1.1 201 Created\r\n"),
		Txn:              driver.HttpTransactionType{RequestMethod: 0, ResponseStatusCode: 0},
	}
	assert.Equal(t, MethodPost, agentMode.Method())
	assert.Equal(t, uint16(201), agentMode.StatusCode())

	legacyMode := WinHttpTransaction{
		ParseInAgent:     false,
		RequestFragment:  []byte("POST /v1/x HTTP/1.1\r\n"),
		ResponseFragment: []byte("HTTP/1.1 201 Created\r\n"),
		Txn:              driver.HttpTransactionType{RequestMethod: uint32(MethodGet), ResponseStatusCode: 200},
	}
	assert.Equal(t, MethodGet, legacyMode.Method())
	assert.Equal(t, uint16(200), legacyMode.StatusCode())
}
