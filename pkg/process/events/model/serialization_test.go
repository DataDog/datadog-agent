// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newBenchmarkProcessEvent(argCount int) *ProcessEvent {
	args := make([]string, 0, argCount)
	for i := 0; i < argCount; i++ {
		args = append(args, fmt.Sprintf("arg_%d", i))
	}

	return NewMockedExitEvent(time.Now(), 42, "/usr/bin/exe", args, 0)
}

// Benchmark between JSON and messagePack serialization changing the command-line length of the collected event
func BenchmarkProcessEventsJSON10(b *testing.B)      { benchmarkProcessEventsJSON(b, 10) }
func BenchmarkProcessEventsMsgPack10(b *testing.B)   { benchmarkProcessEventsMsgPack(b, 10) }
func BenchmarkProcessEventsJSON100(b *testing.B)     { benchmarkProcessEventsJSON(b, 100) }
func BenchmarkProcessEventsMsgPack100(b *testing.B)  { benchmarkProcessEventsMsgPack(b, 100) }
func BenchmarkProcessEventsJSON1000(b *testing.B)    { benchmarkProcessEventsJSON(b, 1000) }
func BenchmarkProcessEventsMsgPack1000(b *testing.B) { benchmarkProcessEventsMsgPack(b, 1000) }

func benchmarkProcessEventsJSON(b *testing.B, argCount int) {
	evt := newBenchmarkProcessEvent(argCount)
	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(evt)
		require.NoError(b, err)

		var desEvt ProcessEvent
		err = json.Unmarshal(data, &desEvt)
		require.NoError(b, err)
	}
}

func benchmarkProcessEventsMsgPack(b *testing.B, argCount int) {
	evt := newBenchmarkProcessEvent(argCount)
	for i := 0; i < b.N; i++ {
		data, err := evt.MarshalMsg(nil)
		require.NoError(b, err)

		var desEvt ProcessEvent
		_, err = desEvt.UnmarshalMsg(data)
		require.NoError(b, err)
	}
}
