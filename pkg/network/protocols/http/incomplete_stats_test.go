// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestOrphanEntries(t *testing.T) {
	t.Run("orphan entries can be joined even after flushing", func(t *testing.T) {
		now := time.Now()
		tel := NewTelemetry("http")
		buffer := NewIncompleteBuffer(config.New(), tel)
		request := &EbpfEvent{
			Http: EbpfTx{
				Request_fragment: requestFragment([]byte("GET /foo/bar")),
				Request_started:  uint64(now.UnixNano()),
			},
		}
		request.Tuple.Sport = 60000

		buffer.Add(request)
		now = now.Add(5 * time.Second)
		complete := buffer.Flush(now)
		assert.Len(t, complete, 0)

		response := &EbpfEvent{
			Http: EbpfTx{
				Response_status_code: 200,
				Response_last_seen:   uint64(now.UnixNano()),
			},
		}
		response.Tuple.Sport = 60000
		buffer.Add(response)
		complete = buffer.Flush(now)
		require.Len(t, complete, 1)

		completeTX := complete[0]
		path, _ := completeTX.Path(make([]byte, 256))
		assert.Equal(t, "/foo/bar", string(path))
		assert.Equal(t, uint16(200), completeTX.StatusCode())
	})

	t.Run("orphan entries are not kept indefinitely", func(t *testing.T) {
		tel := NewTelemetry("http")
		// Temporary cast until we introduce a HTTP2 dedicated implementation for incompleteBuffer.
		buffer := NewIncompleteBuffer(config.New(), tel).(*incompleteBuffer)
		now := time.Now()
		buffer.minAgeNano = (30 * time.Second).Nanoseconds()
		request := &EbpfEvent{
			Http: EbpfTx{
				Request_fragment: requestFragment([]byte("GET /foo/bar")),
				Request_started:  uint64(now.UnixNano()),
			},
		}
		buffer.Add(request)
		_ = buffer.Flush(now)

		assert.True(t, len(buffer.data) > 0)
		now = now.Add(35 * time.Second)
		_ = buffer.Flush(now)
		assert.True(t, len(buffer.data) == 0)
	})
}
