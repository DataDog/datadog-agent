// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package transaction

/*
#include "../../ebpf/c/tracer.h"
#include "../../ebpf/c/tags-types.h"
#include "../../ebpf/c/http-types.h"
*/
import "C"

type HttpConnTuple C.conn_tuple_t
type EbpfHttpTx C.http_transaction_t

const (
	HTTPBatchSize  = C.HTTP_BATCH_SIZE
	HTTPBatchPages = C.HTTP_BATCH_PAGES
	HTTPBufferSize = C.HTTP_BUFFER_SIZE
)

type httpBatch C.http_batch_t
type httpBatchKey C.http_batch_key_t
