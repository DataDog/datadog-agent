// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package http2

/*
#include "../../ebpf/c/conn_tuple.h"
#include "../../ebpf/c/protocols/http2/decoding-defs.h"
*/
import "C"

const (
	maxHTTP2Path     = C.HTTP2_MAX_PATH_LEN
	http2PathBuckets = C.HTTP2_TELEMETRY_PATH_BUCKETS
	// The kernel limit per page in the per-cpu array of the http2 terminated connections map.
	HTTP2TerminatedBatchSize = C.HTTP2_TERMINATED_BATCH_SIZE
	// The upper limit for the size of the raw status code.
	// If the status code is huffman encoded, the size is 2 characters, while if it is not encoded, the size is 3 characters.
	http2RawStatusCodeMaxLength = C.HTTP2_STATUS_CODE_MAX_LEN
	// The max number of headers we process in the request/response.
	Http2MaxHeadersCountPerFiltering = C.HTTP2_MAX_HEADERS_COUNT_FOR_FILTERING
)

type connTuple = C.conn_tuple_t
type HTTP2DynamicTableIndex C.dynamic_table_index_t
type HTTP2DynamicTableEntry C.dynamic_table_entry_t
type http2StreamKey C.http2_stream_key_t
type http2StatusCode C.status_code_t
type http2requestMethod C.method_t
type http2Stream C.http2_stream_t
type http2DynamicTableValue C.dynamic_table_value_t
type EbpfTx C.http2_event_t
type HTTP2Telemetry C.http2_telemetry_t

type StaticTableEnumValue = C.static_table_value_t

const (
	GetValue       StaticTableEnumValue = C.kGET
	PostValue      StaticTableEnumValue = C.kPOST
	EmptyPathValue StaticTableEnumValue = C.kEmptyPath
	IndexPathValue StaticTableEnumValue = C.kIndexPath
	K200Value      StaticTableEnumValue = C.k200
	K204Value      StaticTableEnumValue = C.k204
	K206Value      StaticTableEnumValue = C.k206
	K304Value      StaticTableEnumValue = C.k304
	K400Value      StaticTableEnumValue = C.k400
	K404Value      StaticTableEnumValue = C.k404
	K500Value      StaticTableEnumValue = C.k500
)
