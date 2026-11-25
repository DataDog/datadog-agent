// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Result encoding for the JSON-cell upload format: every CSV field holds exactly
// one JSON value, in descriptor column order.

const (
	remoteFormatJSONCell = "csv-json-cell-v1"

	maxRemoteColumnNameBytes = 255
	maxRemoteTypeNameBytes   = 256
	maxRemoteNumberBytes     = 200
	// remoteRedactedTokenBytes is the size of the `"[REDACTED]"` token intake may
	// substitute for any scanned string or number leaf.
	remoteRedactedTokenBytes = 12
	// remoteMaxPageTargetBytes bounds the agent memory one page may use, whatever the
	// session's larger limits allow.
	remoteMaxPageTargetBytes = 16 << 20
	// remoteMaxSourcePageBytes is intake's uncompressed page limit.
	remoteMaxSourcePageBytes = 100 << 20
)

type remoteKind int

const (
	remoteKindDecimal remoteKind = iota
	remoteKindString
)

type remoteColumn struct {
	Name        string
	VendorType  string
	LogicalType string
	kind        remoteKind
}

// classifyRemoteColumn maps a driver column type to the JSON-cell descriptor. The
// supported set is deliberately small; every other type, including LOBs, dates and
// binary, is rejected rather than guessed.
//
// go-ora reports the wire type NCHAR for VARCHAR2 (and NVARCHAR2) columns, which it
// cannot tell apart; the descriptor names it VARCHAR2.
func classifyRemoteColumn(name, databaseType string) (remoteColumn, *remoteFailure) {
	col := remoteColumn{Name: name}
	switch strings.ToUpper(databaseType) {
	case "NUMBER":
		col.VendorType, col.LogicalType, col.kind = "NUMBER", "decimal", remoteKindDecimal
	case "NCHAR", "VARCHAR2":
		col.VendorType, col.LogicalType, col.kind = "VARCHAR2", "string", remoteKindString
	default:
		return col, newRemoteFailure(remoteCodeUnsupportedColumn, "the result has a column type that remote queries do not support")
	}
	if name == "" || len(name) > maxRemoteColumnNameBytes || !utf8.ValidString(name) {
		return col, newRemoteFailure(remoteCodeInvalidColumns, "a result column name is empty, too long or not valid UTF-8")
	}
	return col, nil
}

// validJSONNumber reports whether s is exactly one JSON number.
func validJSONNumber(s string) bool {
	if len(s) == 0 || len(s) > maxRemoteNumberBytes {
		return false
	}
	i := 0
	if s[i] == '-' {
		i++
	}
	digits := func() int {
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return i - start
	}
	switch {
	case i < len(s) && s[i] == '0':
		i++
	case digits() == 0:
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if digits() == 0 {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if digits() == 0 {
			return false
		}
	}
	return i == len(s)
}

func jsonString(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(s) != nil {
		return "", false
	}
	return strings.TrimSuffix(buf.String(), "\n"), true
}

// encodeRemoteCell returns the JSON token of one driver value and a conservative
// bound on the bytes it can occupy in the stored result file.
//
// NUMBER is never routed through a float: only the driver's exact decimal text (or an
// exact integer) is accepted, and the text is written as the JSON number it is, which
// intake stores as an exact string for a decimal column. A float from the driver is
// rejected because its exactness is unknown.
func encodeRemoteCell(kind remoteKind, value any) (string, int, *remoteFailure) {
	bad := newRemoteFailure(remoteCodeUnsupportedValue, "the result has a value remote queries cannot represent")
	if value == nil {
		return "null", 4, nil
	}
	switch kind {
	case remoteKindDecimal:
		var text string
		switch v := value.(type) {
		case string:
			text = v
		case []byte:
			text = string(v)
		case int64:
			text = strconv.FormatInt(v, 10)
		default:
			return "", 0, bad
		}
		if !validJSONNumber(text) {
			return "", 0, bad
		}
		// Stored as a quoted string of the exact text; a redaction marker may replace it.
		return text, max(len(text)+2, remoteRedactedTokenBytes), nil
	case remoteKindString:
		var text string
		switch v := value.(type) {
		case string:
			text = v
		case []byte:
			text = string(v)
		default:
			return "", 0, bad
		}
		token, ok := jsonString(text)
		if !ok {
			return "", 0, bad
		}
		return token, max(len(token), remoteRedactedTokenBytes), nil
	}
	return "", 0, bad
}

// remoteDescriptor is the registered upload descriptor.
type remoteDescriptor struct {
	FormatVersion string               `json:"format_version"`
	IncludeSchema bool                 `json:"include_schema"`
	AgentHostname string               `json:"agent_hostname"`
	Columns       []remoteDescriptorCl `json:"columns"`
}

type remoteDescriptorCl struct {
	Name        string `json:"column_name"`
	VendorType  string `json:"vendor_data_type"`
	LogicalType string `json:"logical_type"`
}

func newRemoteDescriptor(columns []remoteColumn, includeSchema bool, hostname string) remoteDescriptor {
	d := remoteDescriptor{FormatVersion: remoteFormatJSONCell, IncludeSchema: includeSchema, AgentHostname: hostname}
	for _, c := range columns {
		d.Columns = append(d.Columns, remoteDescriptorCl{Name: c.Name, VendorType: c.VendorType, LogicalType: c.LogicalType})
	}
	return d
}

// schemaJSONBytes is the size of the schema array each result file repeats.
func schemaJSONBytes(columns []remoteColumn) int {
	type entry struct {
		Name       string `json:"column_name"`
		VendorType string `json:"vendor_data_type"`
	}
	entries := make([]entry, len(columns))
	for i, c := range columns {
		entries[i] = entry{c.Name, c.VendorType}
	}
	b, _ := json.Marshal(entries)
	return len(b)
}

// pageEncoder builds one bounded page of CSV records and the conservative size
// bound of the result file intake derives from it.
type pageEncoder struct {
	columns   []remoteColumn
	nameBytes []int
	overhead  int
	body      bytes.Buffer
	writer    *csv.Writer
	rows      int64
	bound     int64 // sum of row bounds, without separators
	fields    []string
}

func newPageEncoder(columns []remoteColumn, includeSchema bool, crawlID, taskID, hostname string) (*pageEncoder, *remoteFailure) {
	e := &pageEncoder{columns: columns, fields: make([]string, len(columns))}
	e.writer = csv.NewWriter(&e.body)
	for _, c := range columns {
		token, ok := jsonString(c.Name)
		if !ok {
			return nil, newRemoteFailure(remoteCodeInvalidColumns, "a result column name is not valid UTF-8")
		}
		e.nameBytes = append(e.nameBytes, len(token)+1)
	}
	host, ok := jsonString(hostname)
	if !ok {
		return nil, newRemoteFailure(remoteCodeInternalError, "the Agent hostname is not valid UTF-8")
	}
	schema := 0
	if includeSchema {
		schema = len(`,"schema":`) + schemaJSONBytes(columns)
	}
	// Header and trailer written around the rows, with the widest record offset and org ID.
	e.overhead = len(fmt.Sprintf(`{"contract_version":"1.0.0","crawl_id":%q,"task_id":%q,"record_offset":`, crawlID, taskID)) + 20 +
		schema + len(`,"data":[`) + len(`],"org_id":`) + 20 + len(`,"agent_hostname":`) + len(host) + len(`,"sensitive_data_scanned":false}`)
	return e, nil
}

// rowBound is the bound on one encoded row's JSON object.
func (e *pageEncoder) rowBound(cellBounds []int) int64 {
	bound := int64(2) // braces
	for i, b := range cellBounds {
		bound += int64(e.nameBytes[i] + b)
		if i > 0 {
			bound++
		}
	}
	return bound
}

// fileBound is the bound on the result file if a row of rowBound were added.
func (e *pageEncoder) fileBound(rowBound int64) int64 {
	n := e.rows + 1
	return int64(e.overhead) + e.bound + rowBound + (n - 1)
}

func (e *pageEncoder) add(tokens []string, rowBound int64) *remoteFailure {
	copy(e.fields, tokens)
	if err := e.writer.Write(e.fields); err != nil {
		return newRemoteFailure(remoteCodeInternalError, "a page record could not be encoded")
	}
	e.writer.Flush()
	e.rows++
	e.bound += rowBound
	return nil
}

func (e *pageEncoder) empty() bool { return e.rows == 0 }

func (e *pageEncoder) emptyBound() int64 { return int64(e.overhead) }

// pageBound is the bound on the result file of the rows added so far.
func (e *pageEncoder) pageBound() int64 {
	if e.rows == 0 {
		return int64(e.overhead)
	}
	return int64(e.overhead) + e.bound + (e.rows - 1)
}

func (e *pageEncoder) bytes() []byte { return e.body.Bytes() }

func (e *pageEncoder) reset() {
	e.body.Reset()
	e.rows, e.bound = 0, 0
}
