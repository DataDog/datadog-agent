// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package ebpf

/*
#include "./c/tracer.h"
#include "./c/http-types.h"
*/
import "C"

type HTTPConnTuple C.conn_tuple_t
type HTTPBatchState C.http_batch_state_t
type SSLSock C.ssl_sock_t
type SSLReadArgs C.ssl_read_args_t
