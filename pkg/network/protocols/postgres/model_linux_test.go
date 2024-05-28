// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"testing"

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
			name:       "extra space between if exists",
			query:      `DROP TABLE  IF  EXISTS test1`,
			tablesName: "test1",
		},
		{
			name:       "single table name with small caps",
			query:      `drop table if exists test1`,
			tablesName: "test1",
		},
		{
			name:       "single table name with mixed caps",
			query:      `drop TablE iF ExISts test1`,
			tablesName: "test1",
		},
		{
			name:       "validate unknown table name",
			query:      `DROP TABLE`,
			tablesName: "UNKNOWN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEventWrapper(&EbpfEvent{
				Tx: EbpfTx{
					Request_fragment:    requestFragment([]byte(tt.query)),
					Original_query_size: uint32(len(tt.query)),
				},
			})
			require.EqualValues(t, e.extractTableName(), tt.tablesName)
		})
	}
}

func BenchmarkExtractTableName(b *testing.B) {
	query := "CREATE TABLE dummy"
	e := NewEventWrapper(&EbpfEvent{
		Tx: EbpfTx{
			Request_fragment:    requestFragment([]byte(query)),
			Original_query_size: uint32(len(query)),
		},
	})
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
