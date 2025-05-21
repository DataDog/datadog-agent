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
	error    RedisErrorType
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
func (e *EventWrapper) ErrorName() RedisErrorType {
	if !e.errorSet {
		e.error = fromEbpfErrorType(errorType(e.Tx.Error))
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

func fromEbpfErrorType(e errorType) RedisErrorType {
	// Map kernel-space error types to user-space RedisErrorType
	switch e {
	case noErr:
		return RedisNoErr
	case unknownErr:
		return RedisErrUnknown
	case err:
		return RedisErrErr
	case wrongType:
		return RedisErrWrongType
	case noAuth:
		return RedisErrNoAuth
	case noPerm:
		return RedisErrNoPerm
	case busy:
		return RedisErrBusy
	case noScript:
		return RedisErrNoScript
	case loading:
		return RedisErrLoading
	case readOnly:
		return RedisErrReadOnly
	case execAbort:
		return RedisErrExecAbort
	case masterDown:
		return RedisErrMasterDown
	case misconf:
		return RedisErrMisconf
	case crossSlot:
		return RedisErrCrossSlot
	case tryAgain:
		return RedisErrTryAgain
	case ask:
		return RedisErrAsk
	case moved:
		return RedisErrMoved
	case clusterDown:
		return RedisErrClusterDown
	case noReplicas:
		return RedisErrNoReplicas
	case oom:
		return RedisErrOom
	case noQuorum:
		return RedisErrNoQuorum
	case busyKey:
		return RedisErrBusyKey
	case unblocked:
		return RedisErrUnblocked
	case unsupported:
		return RedisErrUnsupported
	case syntax:
		return RedisErrSyntax
	case clientClosed:
		return RedisErrClientClosed
	case proxy:
		return RedisErrProxy
	case wrongPass:
		return RedisErrWrongPass
	case invalid:
		return RedisErrInvalid
	case deprecated:
		return RedisErrDeprecated
	default:
		return RedisErrUnknown
	}
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
