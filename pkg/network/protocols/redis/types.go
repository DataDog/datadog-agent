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

type ErrorType C.redis_error_t

const (
	NoErr       = ErrorType(C.REDIS_NO_ERR)
	UnknownErr  = ErrorType(C.REDIS_ERR_UNKNOWN)
	Err         = ErrorType(C.REDIS_ERR_ERR)
	WrongType   = ErrorType(C.REDIS_ERR_WRONGTYPE)
	NoAuth      = ErrorType(C.REDIS_ERR_NOAUTH)
	NoPerm      = ErrorType(C.REDIS_ERR_NOPERM)
	Busy        = ErrorType(C.REDIS_ERR_BUSY)
	NoScript    = ErrorType(C.REDIS_ERR_NOSCRIPT)
	Loading     = ErrorType(C.REDIS_ERR_LOADING)
	ReadOnly    = ErrorType(C.REDIS_ERR_READONLY)
	ExecAbort   = ErrorType(C.REDIS_ERR_EXECABORT)
	MasterDown  = ErrorType(C.REDIS_ERR_MASTERDOWN)
	Misconf     = ErrorType(C.REDIS_ERR_MISCONF)
	CrossSlot   = ErrorType(C.REDIS_ERR_CROSSSLOT)
	TryAgain    = ErrorType(C.REDIS_ERR_TRYAGAIN)
	Ask         = ErrorType(C.REDIS_ERR_ASK)
	Moved       = ErrorType(C.REDIS_ERR_MOVED)
	ClusterDown = ErrorType(C.REDIS_ERR_CLUSTERDOWN)
	NoReplicas  = ErrorType(C.REDIS_ERR_NOREPLICAS)
	Oom         = ErrorType(C.REDIS_ERR_OOM)
	NoQuorum    = ErrorType(C.REDIS_ERR_NOQUORUM)
	BusyKey     = ErrorType(C.REDIS_ERR_BUSYKEY)
	Unblocked   = ErrorType(C.REDIS_ERR_UNBLOCKED)
	WrongPass   = ErrorType(C.REDIS_ERR_WRONGPASS)
	InvalidObj  = ErrorType(C.REDIS_ERR_INVALIDOBJ)
)
