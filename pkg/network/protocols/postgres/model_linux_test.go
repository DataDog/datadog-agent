package postgres

import (
	"testing"
)

func BenchmarkExtractTableName(b *testing.B) {
	query := "CREATE TABLE dummy"
	e := &EventWrapper{
		EbpfEvent: &EbpfEvent{
			Tx: EbpfTx{
				Request_fragment:    requestFragment([]byte(query)),
				Original_query_size: uint32(len(query)),
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.extractTableName()
	}
}

func requestFragment(fragment []byte) [BufferSize]byte {
	if len(fragment) >= BufferSize {
		return *(*[BufferSize]byte)(fragment)
	}
	var b [BufferSize]byte
	copy(b[:], fragment)
	return b
}
