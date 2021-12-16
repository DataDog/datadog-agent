// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"runtime"
	"testing"
)

func BenchmarkBuffer(b *testing.B) {
	var buffer *ConnectionBuffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := NewConnectionBuffer(256, 256)
		for i := 0; i < 512; i++ {
			conn := buffer.Next()
			conn.Pid = uint32(i)
		}
	}
	runtime.KeepAlive(buffer)
}
