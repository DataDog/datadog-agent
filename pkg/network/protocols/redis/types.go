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
	maxCommand     = CommandType(C.__MAX_REDIS_COMMAND)
)

type EbpfEvent C.redis_event_t
type EbpfTx C.redis_transaction_t

type errorType C.redis_error_t

const (
	noErr        = errorType(C.REDIS_NO_ERR)
	unknownErr   = errorType(C.REDIS_ERR_UNKNOWN)
	err          = errorType(C.REDIS_ERR_ERR)
	wrongType    = errorType(C.REDIS_ERR_WRONGTYPE)
	noAuth       = errorType(C.REDIS_ERR_NOAUTH)
	noPerm       = errorType(C.REDIS_ERR_NOPERM)
	busy         = errorType(C.REDIS_ERR_BUSY)
	noScript     = errorType(C.REDIS_ERR_NOSCRIPT)
	loading      = errorType(C.REDIS_ERR_LOADING)
	readOnly     = errorType(C.REDIS_ERR_READONLY)
	execAbort    = errorType(C.REDIS_ERR_EXECABORT)
	masterDown   = errorType(C.REDIS_ERR_MASTERDOWN)
	misconf      = errorType(C.REDIS_ERR_MISCONF)
	crossSlot    = errorType(C.REDIS_ERR_CROSSSLOT)
	tryAgain     = errorType(C.REDIS_ERR_TRYAGAIN)
	ask          = errorType(C.REDIS_ERR_ASK)
	moved        = errorType(C.REDIS_ERR_MOVED)
	clusterDown  = errorType(C.REDIS_ERR_CLUSTERDOWN)
	noReplicas   = errorType(C.REDIS_ERR_NOREPLICAS)
	oom          = errorType(C.REDIS_ERR_OOM)
	noQuorum     = errorType(C.REDIS_ERR_NOQUORUM)
	busyKey      = errorType(C.REDIS_ERR_BUSYKEY)
	unblocked    = errorType(C.REDIS_ERR_UNBLOCKED)
	unsupported  = errorType(C.REDIS_ERR_UNSUPPORTED)
	syntax       = errorType(C.REDIS_ERR_SYNTAX)
	clientClosed = errorType(C.REDIS_ERR_CLIENT_CLOSED)
	proxy        = errorType(C.REDIS_ERR_PROXY)
	wrongPass    = errorType(C.REDIS_ERR_WRONGPASS)
	invalid      = errorType(C.REDIS_ERR_INVALID)
	deprecated   = errorType(C.REDIS_ERR_DEPRECATED)
)
