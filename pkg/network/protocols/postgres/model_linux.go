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

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// EventWrapper wraps an ebpf event and provides additional methods to extract information from it.
// We use this wrapper to avoid recomputing the same values (operation and table name) multiple times.
type EventWrapper struct {
	*EbpfEvent

	operationSet bool
	operation    Operation
	tableNameSet bool
	tableName    string
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
func (e *EbpfTx) getFragment() []byte {
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
		e.operation = FromString(string(bytes.SplitN(e.Tx.getFragment(), []byte(" "), 2)[0]))
		e.operationSet = true
	}
	return e.operation
}

var (
	regexMapper = map[Operation]*regexp.Regexp{
		CreateTableOP: regexp.MustCompile(`CREATE TABLE (\S+)`),
		InsertOP:      regexp.MustCompile(`INSERT INTO (\S+)`),
		DropTableOP:   regexp.MustCompile(`DROP TABLE(\s*IF\s+EXISTS\s+)?(\S+)`),
		UpdateOP:      regexp.MustCompile(`UPDATE (\S+)`),
		SelectOP:      regexp.MustCompile(`SELECT .* FROM (\S+)`),
	}
)

// extractTableName extracts the table name from the query.
func (e *EventWrapper) extractTableName() string {
	regex, ok := regexMapper[e.Operation()]
	if ok {
		if matches := regex.FindSubmatch(e.Tx.getFragment()); len(matches) > 1 {
			return string(
				bytes.Trim( // Remove leading and trailing quotes from the table name
					bytes.ReplaceAll(matches[len(matches)-1], []byte{0}, []byte{}), // Remove null bytes
					"\""),
			)
		}
	}
	return "UNKNOWN"
}

// TableName returns the name of the table the query is operating on.
func (e *EventWrapper) TableName() string {
	if !e.tableNameSet {
		e.tableName = e.extractTableName()
		e.tableNameSet = true
	}

	return e.tableName
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
	output.WriteString(fmt.Sprintf(template, e.Operation(), e.TableName(), e.RequestLatency()))
	return output.String()
}
