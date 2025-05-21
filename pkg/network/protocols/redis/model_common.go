// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package redis

// RedisErrorType represents a Redis error type.
type RedisErrorType int32

const (
	RedisNoErr RedisErrorType = iota
	RedisErrUnknown
	RedisErrErr
	RedisErrWrongType
	RedisErrNoAuth
	RedisErrNoPerm
	RedisErrBusy
	RedisErrNoScript
	RedisErrLoading
	RedisErrReadOnly
	RedisErrExecAbort
	RedisErrMasterDown
	RedisErrMisconf
	RedisErrCrossSlot
	RedisErrTryAgain
	RedisErrAsk
	RedisErrMoved
	RedisErrClusterDown
	RedisErrNoReplicas
	RedisErrOom
	RedisErrNoQuorum
	RedisErrBusyKey
	RedisErrUnblocked
	RedisErrUnsupported
	RedisErrSyntax
	RedisErrClientClosed
	RedisErrProxy
	RedisErrWrongPass
	RedisErrInvalid
	RedisErrDeprecated
)

func (e RedisErrorType) String() string {
	switch e {
	case RedisNoErr:
		return "NO_ERR"
	case RedisErrUnknown:
		return "ERR_UNKNOWN"
	case RedisErrErr:
		return "ERR"
	case RedisErrWrongType:
		return "ERR_WRONGTYPE"
	case RedisErrNoAuth:
		return "ERR_NOAUTH"
	case RedisErrNoPerm:
		return "ERR_NOPERM"
	case RedisErrBusy:
		return "ERR_BUSY"
	case RedisErrNoScript:
		return "ERR_NOSCRIPT"
	case RedisErrLoading:
		return "ERR_LOADING"
	case RedisErrReadOnly:
		return "ERR_READONLY"
	case RedisErrExecAbort:
		return "ERR_EXECABORT"
	case RedisErrMasterDown:
		return "ERR_MASTERDOWN"
	case RedisErrMisconf:
		return "ERR_MISCONF"
	case RedisErrCrossSlot:
		return "ERR_CROSSSLOT"
	case RedisErrTryAgain:
		return "ERR_TRYAGAIN"
	case RedisErrAsk:
		return "ERR_ASK"
	case RedisErrMoved:
		return "ERR_MOVED"
	case RedisErrClusterDown:
		return "ERR_CLUSTERDOWN"
	case RedisErrNoReplicas:
		return "ERR_NOREPLICAS"
	case RedisErrOom:
		return "ERR_OOM"
	case RedisErrNoQuorum:
		return "ERR_NOQUORUM"
	case RedisErrBusyKey:
		return "ERR_BUSYKEY"
	case RedisErrUnblocked:
		return "ERR_UNBLOCKED"
	case RedisErrUnsupported:
		return "ERR_UNSUPPORTED"
	case RedisErrSyntax:
		return "ERR_SYNTAX"
	case RedisErrClientClosed:
		return "ERR_CLIENT_CLOSED"
	case RedisErrProxy:
		return "ERR_PROXY"
	case RedisErrWrongPass:
		return "ERR_WRONGPASS"
	case RedisErrInvalid:
		return "ERR_INVALID"
	case RedisErrDeprecated:
		return "ERR_DEPRECATED"
	default:
		return "UNKNOWN_ERROR"
	}
}
