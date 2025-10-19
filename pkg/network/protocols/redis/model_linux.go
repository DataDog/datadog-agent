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
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

// EventWrapper wraps an ebpf event and provides additional methods to extract information from it.
// We use this wrapper to avoid recomputing the same values (key name) multiple times.
type EventWrapper struct {
	*EbpfEvent
	*EbpfKey

	keyNameSet bool
	keyName    *intern.StringValue
	commandSet bool
	command    CommandType
}

// NewEventWrapper creates a new EventWrapper from an ebpf event.
func NewEventWrapper(e *EbpfEvent, k *EbpfKey) *EventWrapper {
	return &EventWrapper{EbpfEvent: e, EbpfKey: k}
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
func getFragment(e *EbpfKey) []byte {
	if e.Len == 0 {
		return nil
	}
	if e.Len > uint16(len(e.Buf)) {
		return e.Buf[:len(e.Buf)]
	}
	return e.Buf[:e.Len]
}

// KeyName returns the key name of the key.
func (e *EventWrapper) KeyName() *intern.StringValue {
	if !e.keyNameSet && e.EbpfKey != nil {
		e.keyName = Interner.Get(getFragment(e.EbpfKey))
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
	if e.Tx.Request_started == 0 || e.Tx.Response_last_seen == 0 {
		return 0
	}
	if e.Tx.Response_last_seen < e.Tx.Request_started {
		return 0
	}
	return protocols.NSTimestampToFloat(e.Tx.Response_last_seen - e.Tx.Request_started)
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
	if e.EbpfKey != nil && e.Truncated {
		truncatedPath = " (truncated)"
	}
	output.WriteString(fmt.Sprintf(template, e.CommandType(), e.KeyName().Get(), truncatedPath, e.RequestLatency()))
	return output.String()
}

// String returns a string representation of Command
func (c CommandType) String() string {
	switch c {
	case GetCommand:
		return "GET"
	case SetCommand:
		return "SET"
	case PingCommand:
		return "PING"
	default:
		return "UNKNOWN"
	}
}
