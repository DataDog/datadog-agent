// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package gotls

/*
#include "../../ebpf/c/protocols/tls/go-tls-types.h"
*/
import "C"

type Location C.location_t
type SliceLocation C.slice_location_t
type GoroutineIDMetadata C.goroutine_id_metadata_t
type TlsBinaryId C.go_tls_offsets_data_key_t
type TlsConnLayout C.tls_conn_layout_t
type TlsOffsetsData C.tls_offsets_data_t
