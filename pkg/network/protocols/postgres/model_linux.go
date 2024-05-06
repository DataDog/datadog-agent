// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// ConnTuple returns the connection tuple for the transaction
func (e *EbpfEvent) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: e.Tuple.Saddr_h,
		SrcIPLow:  e.Tuple.Saddr_l,
		DstIPHigh: e.Tuple.Daddr_h,
		DstIPLow:  e.Tuple.Daddr_l,
		SrcPort:   e.Tuple.Sport,
		DstPort:   e.Tuple.Dport,
	}
}

// Operation returns the operation of the query (SELECT, INSERT, UPDATE, DELETE, etc.)
func (e *EbpfEvent) Operation() string {
	return string(bytes.SplitN(e.Tx.Request_fragment[:e.Tx.Frag_size], []byte(" "), 2)[0])
}

var (
	createPattern = regexp.MustCompile(`CREATE TABLE (\S+)`)
	insertPattern = regexp.MustCompile(`INSERT INTO (\S+)`)
	dropPattern   = regexp.MustCompile(`DROP TABLE (\S+)`)
	updatePattern = regexp.MustCompile(`UPDATE (\S+)`)
	selectPattern = regexp.MustCompile(`SELECT .* FROM (\S+)`)
)

// TableName returns the name of the table the query is operating on.
func (e *EbpfEvent) TableName() string {
	b := e.Tx.Request_fragment[:e.Tx.Frag_size]
	if matches := createPattern.FindSubmatch(e.Tx.Request_fragment[:e.Tx.Frag_size]); len(matches) > 1 {
		return string(bytes.Trim(bytes.ReplaceAll(matches[1], []byte{0}, []byte{}), "\""))
	}
	if matches := insertPattern.FindSubmatch(e.Tx.Request_fragment[:e.Tx.Frag_size]); len(matches) > 1 {
		return string(bytes.Trim(bytes.ReplaceAll(matches[1], []byte{0}, []byte{}), "\""))
	}
	if matches := dropPattern.FindSubmatch(e.Tx.Request_fragment[:e.Tx.Frag_size]); len(matches) > 1 {
		return string(bytes.Trim(bytes.ReplaceAll(matches[1], []byte{0}, []byte{}), "\""))
	}
	if matches := updatePattern.FindSubmatch(e.Tx.Request_fragment[:e.Tx.Frag_size]); len(matches) > 1 {
		return string(bytes.Trim(bytes.ReplaceAll(matches[1], []byte{0}, []byte{}), "\""))
	}
	if matches := selectPattern.FindSubmatch(e.Tx.Request_fragment[:e.Tx.Frag_size]); len(matches) > 1 {
		return string(bytes.Trim(bytes.ReplaceAll(matches[1], []byte{0}, []byte{}), "\""))
	}

	log.Debug("No match found for request fragment: ", b)
	// Return an empty string if no match is found
	return ""
}

// RequestLatency returns the latency of the request in nanoseconds
func (e *EbpfEvent) RequestLatency() float64 {
	if uint64(e.Tx.Request_started) == 0 || uint64(e.Tx.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(e.Tx.Response_last_seen - e.Tx.Request_started)
}

const template = `
ebpfTx{
	Operation: '%s',
	Table name: '%s',
	Latency: %f
}`

// String returns a string representation of the underlying event
func (e *EbpfEvent) String() string {
	var output strings.Builder
	output.WriteString(fmt.Sprintf(template, e.Operation(), e.TableName(), e.RequestLatency()))
	return output.String()
}
