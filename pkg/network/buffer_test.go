package network

import (
	"runtime"
	"testing"
)

func BenchmarkBuffer(b *testing.B) {
	var buffer *Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := NewBuffer(256)
		for i := 0; i < 512; i++ {
			conn := buffer.Next()
			conn.Pid = uint32(i)
		}
	}
	runtime.KeepAlive(buffer)
}
