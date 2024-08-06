// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package ebpfpostgres

/*
#include "../../ebpf/c/protocols/postgres/types.h"
#include "../../ebpf/c/protocols/classification/defs.h"
*/
import "C"

// This package was created to use BufferSize from the config package while avoiding cyclic imports.

type ConnTuple = C.conn_tuple_t

type EbpfEvent C.postgres_event_t
type EbpfTx C.postgres_transaction_t

const (
	BufferSize = C.POSTGRES_BUFFER_SIZE
)

// GetFragment returns the actual query fragment from the event.
func (e *EbpfTx) GetFragment() []byte {
	if e.Original_query_size == 0 {
		return nil
	}
	if e.Original_query_size > uint32(len(e.Request_fragment)) {
		return e.Request_fragment[:len(e.Request_fragment)]
	}
	return e.Request_fragment[:e.Original_query_size]
}
