// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/stretchr/testify/assert"
)

func TestIncompleteBuffer(t *testing.T) {
	t.Run("becoming complete", func(t *testing.T) {
		// Testing the scenario where an incomplete request becomes complete.
		buffer := NewIncompleteBuffer(config.New()).(*incompleteBuffer)
		now := time.Now()
		buffer.minAgeNano = (30 * time.Second).Nanoseconds()
		request := &EbpfTx{
			Tuple: connTuple{
				Sport: 6000,
			},
			Stream: http2Stream{
				Response_last_seen: 0, // Required to make the request incomplete.
				Request_started:    uint64(now.UnixNano()),
				Status_code: http2StatusCode{
					Static_table_entry: K200Value,
					Finalized:          true,
				},
				Request_method: http2requestMethod{
					Static_table_entry: GetValue,
					Finalized:          true,
				},
				Path: http2Path{
					Static_table_entry: EmptyPathValue,
					Finalized:          true,
				},
			},
		}
		buffer.Add(request)
		transactions := buffer.Flush(now)
		require.Empty(t, transactions)
		assert.True(t, len(buffer.data) == 1)

		buffer.data[0].Stream.Response_last_seen = uint64(now.Add(time.Second).UnixNano())
		transactions = buffer.Flush(now)
		require.Len(t, transactions, 1)
		assert.True(t, len(buffer.data) == 0)
	})

	t.Run("removing old incomplete", func(t *testing.T) {
		// Testing the scenario where an incomplete request is removed after a certain time.
		buffer := NewIncompleteBuffer(config.New()).(*incompleteBuffer)
		now := time.Now()
		buffer.minAgeNano = (30 * time.Second).Nanoseconds()
		request := &EbpfTx{
			Tuple: connTuple{
				Sport: 6000,
			},
			Stream: http2Stream{
				Path: http2Path{
					Static_table_entry: EmptyPathValue,
				},
				Request_started: uint64(now.UnixNano()),
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
