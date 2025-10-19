// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package redis

/*
#include "../../ebpf/c/protocols/redis/types.h"
#include "../../ebpf/c/protocols/classification/defs.h"
*/
import "C"

type ConnTuple = C.conn_tuple_t

type CommandType C.redis_command_t

var (
	UnknownCommand = CommandType(C.REDIS_UNKNOWN)
	GetCommand     = CommandType(C.REDIS_GET)
	SetCommand     = CommandType(C.REDIS_SET)
	PingCommand    = CommandType(C.REDIS_PING)
	maxCommand     = CommandType(C.__MAX_REDIS_COMMAND)
)

type EbpfEvent C.redis_event_t
type EbpfKeyedEvent C.redis_with_key_event_t
type EbpfKey C.redis_key_data_t
type EbpfTx C.redis_transaction_t
