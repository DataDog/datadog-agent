// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/DataDog/go-sqllexer"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EmptyParameters represents the case where the non-empty query has no parameters
	EmptyParameters = "EMPTY_PARAMETERS"
)

var (
	postgresDBMS = sqllexer.WithDBMS(sqllexer.DBMSPostgres)
)

// EventWrapper wraps an ebpf event and provides additional methods to extract information from it.
// We use this wrapper to avoid recomputing the same values (operation and table name) multiple times.
type EventWrapper struct {
	*ebpf.EbpfEvent

	operationSet  bool
	operation     Operation
	parametersSet bool
	parameters    string
	normalizer    *sqllexer.Normalizer
}

// NewEventWrapper creates a new EventWrapper from an ebpf event.
func NewEventWrapper(e *ebpf.EbpfEvent) *EventWrapper {
	return &EventWrapper{
		EbpfEvent:  e,
		normalizer: sqllexer.NewNormalizer(sqllexer.WithCollectTables(true)),
	}
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
func getFragment(e *ebpf.EbpfTx) []byte {
	if e.Original_query_size == 0 {
		return nil
	}
	if e.Original_query_size > uint32(len(e.Request_fragment)) {
		return e.Request_fragment[:len(e.Request_fragment)]
	}
	return e.Request_fragment[:e.Original_query_size]
}

// Operation returns the operation of the query (SELECT, INSERT, UPDATE, DROP, etc.)
func (e *EventWrapper) Operation() Operation {
	if !e.operationSet {
		op, _, _ := bytes.Cut(getFragment(&e.Tx), []byte(" "))
		e.operation = FromString(string(op))
		e.operationSet = true
	}
	return e.operation
}

// extractParameters returns the string following the command
func (e *EventWrapper) extractParameters() string {
	b := getFragment(&e.Tx)
	idxParam := bytes.IndexByte(b, ' ') // trim the string to a space, it will give the parameter
	if idxParam == -1 {
		return EmptyParameters
	}
	idxParam++

	idxEnd := bytes.IndexByte(b[idxParam:], '\x00') // trim trailing nulls
	if idxEnd == 0 {
		return EmptyParameters
	}
	if idxEnd != -1 {
		return string(b[idxParam : idxParam+idxEnd])
	}
	return string(b[idxParam:])
}

// extractTableName extracts the table name from the query.
func (e *EventWrapper) extractTableName() string {
	// Normalize the query without obfuscating it.
	_, statementMetadata, err := e.normalizer.Normalize(string(getFragment(&e.Tx)), postgresDBMS)
	if err != nil {
		log.Debugf("unable to normalize due to: %s", err)
		return "UNKNOWN"
	}
	if statementMetadata.Size == 0 {
		return "UNKNOWN"
	}

	// Currently, we do not support complex queries with multiple tables. Therefore, we will return only a single table.
	return statementMetadata.Tables[0]

}

// Parameters returns the table name or run-time parameter.
func (e *EventWrapper) Parameters() string {
	if !e.parametersSet {
		if e.operation == ShowOP {
			e.parameters = e.extractParameters()
		} else {
			e.parameters = e.extractTableName()
		}
		e.parametersSet = true
	}

	return e.parameters
}

// RequestLatency returns the latency of the request in nanoseconds
func (e *EventWrapper) RequestLatency() float64 {
	if uint64(e.Tx.Request_started) == 0 || uint64(e.Tx.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(e.Tx.Response_last_seen - e.Tx.Request_started)
}

const template = `
ebpfTx{
	Operation: %q,
	Table Name: %q,
	Latency: %f
}`

// String returns a string representation of the underlying event
func (e *EventWrapper) String() string {
	var output strings.Builder
	output.WriteString(fmt.Sprintf(template, e.Operation(), e.Parameters(), e.RequestLatency()))
	return output.String()
}
