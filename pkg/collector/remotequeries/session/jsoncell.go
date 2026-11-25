// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FormatJSONCell is intake's JSON-cell upload format: each CSV field holds exactly one
// JSON value (intake INPUT-01).
const FormatJSONCell = "csv-json-cell-v1"

// Logical types a column can have (intake DESC-02).
const (
	LogicalBoolean  = "boolean"
	LogicalInteger  = "integer"
	LogicalDecimal  = "decimal"
	LogicalFloat    = "float"
	LogicalString   = "string"
	LogicalTemporal = "temporal"
	LogicalJSON     = "json"
	LogicalBinary   = "binary"
	LogicalVendor   = "vendor"
)

const (
	maxColumnNameBytes = 255
	maxTypeNameBytes   = 256
	maxNumberBytes     = 200
	// redactedTokenBytes is the size of the `"[REDACTED]"` token intake may put in
	// place of a scanned value.
	redactedTokenBytes = 12
	// maxFloatTokenBytes bounds how intake writes a JSON number back as a 64-bit float.
	maxFloatTokenBytes = 24
)

// Column is one result column of the descriptor.
type Column struct {
	Name         string
	DatabaseType string
	LogicalType  string
}

// Cell is one encoded field: its JSON token and a bound on the bytes the value can take
// in the result file.
type Cell struct {
	Token string
	Bound int
}

// Null is the cell of an SQL null.
var Null = Cell{Token: "null", Bound: 4}

// StringCell encodes a string value. It fails when the value is not valid UTF-8.
func StringCell(s string) (Cell, bool) {
	token, ok := jsonString(s)
	if !ok {
		return Cell{}, false
	}
	return Cell{Token: token, Bound: max(len(token), redactedTokenBytes)}, true
}

// DecimalCell encodes the exact text of a decimal number. Intake stores it as a string
// holding that text (intake INPUT-07). It fails when the text is not one JSON number.
func DecimalCell(text string) (Cell, bool) {
	if !ValidJSONNumber(text) {
		return Cell{}, false
	}
	return Cell{Token: text, Bound: max(len(text)+2, redactedTokenBytes)}, true
}

// FloatCell encodes a number of a float column; intake stores it as a 64-bit float.
// NaN and infinities, which JSON numbers cannot hold, are the strings "NaN",
// "Infinity", and "-Infinity".
func FloatCell(text string) (Cell, bool) {
	switch text {
	case "NaN", "Infinity", "-Infinity":
		c, _ := StringCell(text)
		return c, true
	}
	if !ValidJSONNumber(text) {
		return Cell{}, false
	}
	return Cell{Token: text, Bound: max(len(text), maxFloatTokenBytes)}, true
}

// ValidJSONNumber reports whether s is exactly one JSON number of at most 200 bytes.
func ValidJSONNumber(s string) bool {
	if len(s) == 0 || len(s) > maxNumberBytes {
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

// ValidateColumns checks that a descriptor can carry the result's columns: at least
// one, at most the column limit, names and type names that intake accepts, no two with
// the same name, and, when the schema is included, a schema within its size limit.
func ValidateColumns(columns []Column, limits Limits, includeSchema bool) *Failure {
	if len(columns) == 0 || int64(len(columns)) > limits.MaxColumns {
		return Fail(CodeInvalidColumns, "the result has no columns or more than the column limit of "+strconv.FormatInt(limits.MaxColumns, 10))
	}
	seen := make(map[string]struct{}, len(columns))
	for _, c := range columns {
		if c.Name == "" || len(c.Name) > maxColumnNameBytes || !utf8.ValidString(c.Name) {
			return Fail(CodeInvalidColumns, "a result column name is empty, longer than 255 bytes, or not valid UTF-8")
		}
		if c.DatabaseType == "" || len(c.DatabaseType) > maxTypeNameBytes {
			return Fail(CodeInvalidColumns, "the type name of column "+quoteName(c.Name)+" is empty or too long")
		}
		if _, dup := seen[c.Name]; dup {
			return Fail(CodeInvalidColumns, "the result has two columns named "+quoteName(c.Name)+"; give each column a unique alias")
		}
		seen[c.Name] = struct{}{}
	}
	if includeSchema && int64(schemaBytes(columns)) > limits.MaxSchemaBytes {
		return Fail(CodeSchemaTooLarge, "the result schema is larger than the schema size limit")
	}
	return nil
}

func quoteName(name string) string {
	token, _ := jsonString(name)
	return token
}

type descriptor struct {
	FormatVersion string             `json:"format_version"`
	IncludeSchema bool               `json:"include_schema"`
	AgentHostname string             `json:"agent_hostname"`
	Columns       []descriptorColumn `json:"columns"`
}

type descriptorColumn struct {
	Name        string `json:"column_name"`
	VendorType  string `json:"vendor_data_type"`
	LogicalType string `json:"logical_type"`
}

func descriptorJSON(columns []Column, includeSchema bool, hostname string) ([]byte, error) {
	d := descriptor{FormatVersion: FormatJSONCell, IncludeSchema: includeSchema, AgentHostname: hostname}
	for _, c := range columns {
		d.Columns = append(d.Columns, descriptorColumn{Name: c.Name, VendorType: c.DatabaseType, LogicalType: c.LogicalType})
	}
	return json.Marshal(d)
}

// schemaBytes is the size of the schema array each result file repeats.
func schemaBytes(columns []Column) int {
	type entry struct {
		Name       string `json:"column_name"`
		VendorType string `json:"vendor_data_type"`
	}
	entries := make([]entry, len(columns))
	for i, c := range columns {
		entries[i] = entry{c.Name, c.DatabaseType}
	}
	b, _ := json.Marshal(entries)
	return len(b)
}
