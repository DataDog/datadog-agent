// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/go-sqllexer"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventWrapper wraps an ebpf event and provides additional methods to extract information from it.
// We use this wrapper to avoid recomputing the same values (operation and table name) multiple times.
type EventWrapper struct {
	*ebpf.EbpfEvent

	operationSet bool
	operation    Operation
	operandSet   bool
	operand      string
	normalizer   *sqllexer.Normalizer
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
		e.operation = FromString(string(bytes.SplitN(getFragment(&e.Tx), []byte(" "), 2)[0]))
		e.operationSet = true
	}
	return e.operation
}

// extractOperand returns the string following the command
func (e *EventWrapper) extractParameters() string {
	b := getFragment(&e.Tx)
	idxStart := bytes.Index(b, []byte(" ")) + 1 // trim operation
	idxEnd := bytes.Index(b, []byte("\x00"))    // trim trailing nulls
	return string(b[idxStart:idxEnd])
}

var re = regexp.MustCompile(`(?i)if\s+exists`)

// extractTableName extracts the table name from the query.
func (e *EventWrapper) extractTableName() string {
	fragment := string(getFragment(&e.Tx))
	// Temp solution for the fact that ObfuscateSQLString does not support "IF EXISTS" or "if exists", so we remove
	// it from the fragment if found.
	fragment = re.ReplaceAllString(fragment, "")

	// Normalize the query without obfuscating it.
	_, statementMetadata, err := e.normalizer.Normalize(fragment, sqllexer.WithDBMS(sqllexer.DBMSPostgres))
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
	if !e.operandSet {
		if e.operation == ShowOP {
			e.operand = e.extractParameters()
		} else {
			e.operand = e.extractTableName()
		}
		e.operandSet = true
	}

	return e.operand
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
