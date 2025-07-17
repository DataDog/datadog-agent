// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package http

/*
#include "../../ebpf/c/protocols/http/types.h"
#include "../../ebpf/c/protocols/classification/defs.h"
*/
import "C"

type ConnTuple = C.conn_tuple_t
type SslSock C.ssl_sock_t
type SslReadArgs C.ssl_read_args_t
type SslReadExArgs C.ssl_read_ex_args_t
type SslWriteArgs C.ssl_write_args_t
type SslWriteExArgs C.ssl_write_ex_args_t

type EbpfEvent C.http_event_t
type EbpfTx C.http_transaction_t

const (
	BufferSize = C.HTTP_BUFFER_SIZE
)
