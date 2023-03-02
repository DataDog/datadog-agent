// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package http

/*
#include "../../ebpf/c/tracer.h"
#include "../../ebpf/c/protocols/http2/decoding-defs.h"
*/
import "C"

type http2ConnTuple = C.conn_tuple_t
type ebpfHttp2Tx C.http2_stream_t

type StaticTableEnumKey = C.static_table_key_t

const (
	MethodKey StaticTableEnumKey = C.kMethod
	PathKey   StaticTableEnumKey = C.kPath
	StatusKey StaticTableEnumKey = C.kStatus
)

type StaticTableEnumValue = C.static_table_key_t

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

type StaticTableValue = C.static_table_entry_t
