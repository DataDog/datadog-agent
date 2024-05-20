// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"testing"

	"github.com/DataDog/go-sqllexer"
	"github.com/stretchr/testify/require"
)

func TestExtractTableFunction(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		tablesName string
	}{
		{
			name:       "single table name",
			query:      `DROP TABLE IF EXISTS test1`,
			tablesName: "test1",
		},
		{
			name:       "multiple table names",
			query:      `DROP TABLE IF EXISTS test1;DROP TABLE IF EXISTS test2;`,
			tablesName: "test1,test2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &EventWrapper{
				EbpfEvent: &EbpfEvent{
					Tx: EbpfTx{
						Request_fragment:    requestFragment([]byte(tt.query)),
						Original_query_size: uint32(len(tt.query)),
					},
				},
				normalizer: sqllexer.NewNormalizer(
					sqllexer.WithCollectTables(true),
				),
			}
			require.EqualValues(t, e.extractTableName(), tt.tablesName)
		})
	}
}

func BenchmarkExtractTableName(b *testing.B) {
	query := "CREATE TABLE dummy"
	e := &EventWrapper{
		EbpfEvent: &EbpfEvent{
			Tx: EbpfTx{
				Request_fragment:    requestFragment([]byte(query)),
				Original_query_size: uint32(len(query)),
			},
		},
		normalizer: sqllexer.NewNormalizer(
			sqllexer.WithCollectTables(true),
		),
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
