// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// EventWrapper wraps an ebpf event and provides additional methods to extract information from it.
// We use this wrapper to avoid recomputing the same values (key name) multiple times.
type EventWrapper struct {
	*EbpfEvent

	keyNameSet bool
	keyName    string
	commandSet bool
	command    CommandType

	errorSet bool
	error    ErrorType
}

// NewEventWrapper creates a new EventWrapper from an ebpf event.
func NewEventWrapper(e *EbpfEvent) *EventWrapper {
	return &EventWrapper{EbpfEvent: e}
}

// ConnTuple returns the connection tuple for the transaction
func (e *EventWrapper) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: e.Tuple.Saddr_h,
		SrcIPLow:  e.Tuple.Saddr_l,
		DstIPHigh: e.Tuple.Daddr_h,
		DstIPLow:  e.Tuple.Daddr_l,
		SrcPort:   e.Tuple.Sport,
		DstPort:   e.Tuple.Dport,
	}
}

// getFragment returns the actual query fragment from the event.
func getFragment(e *EbpfTx) []byte {
	if e.Buf_len == 0 {
		return nil
	}
	if e.Buf_len > uint16(len(e.Buf)) {
		return e.Buf[:len(e.Buf)]
	}
	return e.Buf[:e.Buf_len]
}

// KeyName returns the key name of the key.
func (e *EventWrapper) KeyName() string {
	if !e.keyNameSet {
		e.keyName = string(getFragment(&e.Tx))
		e.keyNameSet = true
	}
	return e.keyName
}

// CommandType returns the command type of the query
func (e *EventWrapper) CommandType() CommandType {
	if !e.commandSet {
		e.command = CommandType(e.Tx.Command)
		e.commandSet = true
	}
	return e.command
}

// RequestLatency returns the latency of the request in nanoseconds
func (e *EventWrapper) RequestLatency() float64 {
	if uint64(e.Tx.Request_started) == 0 || uint64(e.Tx.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(e.Tx.Response_last_seen - e.Tx.Request_started)
}

// ErrorName returns the error name as a string, extracted from the null-terminated Error field.
func (e *EventWrapper) ErrorName() ErrorType {
	if !e.errorSet {
		e.error = ErrorType(e.Tx.Error)
		e.errorSet = true
	}
	return e.error
}

const template = `
ebpfTx{
	Command: %s,
	Key: %s%s,
	Latency: %f
}`

// String returns a string representation of the underlying event
func (e *EventWrapper) String() string {
	var output strings.Builder
	var truncatedPath string
	if e.Tx.Truncated {
		truncatedPath = " (truncated)"
	}
	output.WriteString(fmt.Sprintf(template, e.CommandType(), e.KeyName(), truncatedPath, e.RequestLatency()))
	return output.String()
}

// String returns a string representation of Command
func (c CommandType) String() string {
	switch c {
	case GetCommand:
		return "GET"
	case SetCommand:
		return "SET"
	default:
		return "UNKNOWN"
	}
}

func (e ErrorType) String() string {
	switch e {
	case NoErr:
		return "NO_ERR"
	case UnknownErr:
		return "ERR_UNKNOWN"
	case GenericErr:
		return "ERR"
	case WrongType:
		return "ERR_WRONGTYPE"
	case NoAuth:
		return "ERR_NOAUTH"
	case NoPerm:
		return "ERR_NOPERM"
	case Busy:
		return "ERR_BUSY"
	case NoScript:
		return "ERR_NOSCRIPT"
	case Loading:
		return "ERR_LOADING"
	case ReadOnly:
		return "ERR_READONLY"
	case ExecAbort:
		return "ERR_EXECABORT"
	case MasterDown:
		return "ERR_MASTERDOWN"
	case Misconf:
		return "ERR_MISCONF"
	case CrossSlot:
		return "ERR_CROSSSLOT"
	case TryAgain:
		return "ERR_TRYAGAIN"
	case Ask:
		return "ERR_ASK"
	case Moved:
		return "ERR_MOVED"
	case ClusterDown:
		return "ERR_CLUSTERDOWN"
	case NoReplicas:
		return "ERR_NOREPLICAS"
	case Oom:
		return "ERR_OOM"
	case NoQuorum:
		return "ERR_NOQUORUM"
	case BusyKey:
		return "ERR_BUSYKEY"
	case Unblocked:
		return "ERR_UNBLOCKED"
	case WrongPass:
		return "ERR_WRONGPASS"
	case InvalidObj:
		return "ERR_INVALIDOBJ"
	default:
		return "ERR_UNKNOWN"
	}
}
