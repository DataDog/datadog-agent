// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrphanEntries(t *testing.T) {
	t.Run("orphan entries can be joined even after flushing", func(t *testing.T) {
		now := time.Now()
		buffer := newIncompleteBuffer(config.New(), newTelemetry())
		request := httpTX{
			request_fragment: requestFragment([]byte("GET /foo/bar")),
			request_started:  _Ctype_ulonglong(now.UnixNano()),
		}
		request.tup.sport = 60000

		buffer.Add(request)
		now = now.Add(5 * time.Second)
		complete := buffer.Flush(now)
		assert.Len(t, complete, 0)

		response := httpTX{
			response_status_code: 200,
			response_last_seen:   _Ctype_ulonglong(now.UnixNano()),
		}
		response.tup.sport = 60000
		buffer.Add(response)
		complete = buffer.Flush(now)
		require.Len(t, complete, 1)

		completeTX := complete[0]
		assert.Equal(t, "/foo/bar", string(getPath(completeTX.ReqFragment(), make([]byte, 256))))
		assert.Equal(t, 200, completeTX.StatusClass())
	})

	t.Run("orphan entries are not kept indefinitely", func(t *testing.T) {
		buffer := newIncompleteBuffer(config.New(), newTelemetry())
		now := time.Now()
		buffer.minAgeNano = (30 * time.Second).Nanoseconds()
		request := httpTX{
			request_fragment: requestFragment([]byte("GET /foo/bar")),
			request_started:  _Ctype_ulonglong(now.UnixNano()),
		}
		buffer.Add(request)
		_ = buffer.Flush(now)

		assert.True(t, len(buffer.data) > 0)
		now = now.Add(35 * time.Second)
		_ = buffer.Flush(now)
		assert.True(t, len(buffer.data) == 0)
	})
}

func requestFragment(fragment []byte) [HTTPBufferSize]_Ctype_char {
	var b [HTTPBufferSize]_Ctype_char
	for i := 0; i < len(b) && i < len(fragment); i++ {
		b[i] = _Ctype_char(fragment[i])
	}
	return b
}
