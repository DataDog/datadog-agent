// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package http

/*
#include "../ebpf/c/tracer.h"
#include "../ebpf/c/http-types.h"
*/
import "C"

type httpConnTuple C.conn_tuple_t
type httpBatchState C.http_batch_state_t
type sslSock C.ssl_sock_t
type sslReadArgs C.ssl_read_args_t

type httpTX C.http_transaction_t
type httpNotification C.http_batch_notification_t
type httpBatch C.http_batch_t
type httpBatchKey C.http_batch_key_t

type libPath C.lib_path_t

const (
	HTTPBatchSize  = C.HTTP_BATCH_SIZE
	HTTPBatchPages = C.HTTP_BATCH_PAGES
	HTTPBufferSize = C.HTTP_BUFFER_SIZE

	httpProg = C.HTTP_PROG

	libPathMaxSize = C.LIB_PATH_MAX_SIZE
)
