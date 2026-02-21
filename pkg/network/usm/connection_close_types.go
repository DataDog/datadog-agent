// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ignore

package usm

/*
#include "../ebpf/c/conn_tuple.h"
#include "../ebpf/c/protocols/classification/defs.h"
#include "../ebpf/c/protocols/tls/connection-close.h"
*/
import "C"

type ConnTuple = C.conn_tuple_t
type ProtocolStack = C.protocol_stack_t

type EbpfConnectionCloseEvent C.tcp_close_event_t
