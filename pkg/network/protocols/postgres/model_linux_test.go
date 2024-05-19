package postgres

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
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
		oq: obfuscate.NewObfuscator(obfuscate.Config{
			SQL: obfuscate.SQLConfig{
				DBMS:            obfuscate.DBMSPostgres,
				ObfuscationMode: obfuscate.NormalizeOnly,
				TableNames:      true,
			}}),
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
