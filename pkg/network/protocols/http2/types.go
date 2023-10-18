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
	maxHTTP2Path = C.HTTP2_MAX_PATH_LEN
)

type connTuple = C.conn_tuple_t
type http2DynamicTableIndex C.dynamic_table_index_t
type http2DynamicTableEntry C.dynamic_table_entry_t
type http2StreamKey C.http2_stream_key_t
type EbpfTx C.http2_stream_t

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
