// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries/session"
)

// remoteKind is how a column's values are encoded (annex: column types).
type remoteKind int

const (
	kindDecimal remoteKind = iota
	kindFloat
	kindString
	kindDate
	kindTimestamp
	kindTimestampTZ
	kindTimestampUTC
	kindInterval
	kindBinary
)

// remoteColumn is one result column: its descriptor entry and value encoding.
type remoteColumn struct {
	session.Column
	kind remoteKind
}

// remoteColumnType describes a column from the go-ora driver's type name, and its
// precision and scale for NUMBER. It fails for types the annex does not list.
func remoteColumnType(name, driverType string, precision, scale int64, hasScale bool) (remoteColumn, *session.Failure) {
	col := remoteColumn{Column: session.Column{Name: name}}
	set := func(dbType, logical string, kind remoteKind) {
		col.DatabaseType, col.LogicalType, col.kind = dbType, logical, kind
	}
	switch driverType {
	case "NUMBER":
		set(numberTypeName(precision, scale, hasScale), session.LogicalDecimal, kindDecimal)
	case "IBFloat", "BINARY_FLOAT":
		set("BINARY_FLOAT", session.LogicalFloat, kindFloat)
	case "IBDouble", "BINARY_DOUBLE":
		set("BINARY_DOUBLE", session.LogicalFloat, kindFloat)
	case "NCHAR", "VARCHAR2", "NVARCHAR2":
		// go-ora names VARCHAR2 and NVARCHAR2 columns NCHAR.
		set("VARCHAR2", session.LogicalString, kindString)
	case "CHAR":
		set("CHAR", session.LogicalString, kindString)
	case "ROWID", "UROWID":
		set(driverType, session.LogicalString, kindString)
	case "DATE":
		set("DATE", session.LogicalTemporal, kindDate)
	case "TimeStampDTY", "TimeStamp", "TIMESTAMP":
		set("TIMESTAMP", session.LogicalTemporal, kindTimestamp)
	case "TimeStampTZ_DTY", "TimeStampTZ":
		set("TIMESTAMP WITH TIME ZONE", session.LogicalTemporal, kindTimestampTZ)
	case "TimeStampLTZ_DTY", "TimeStampLTZ":
		set("TIMESTAMP WITH LOCAL TIME ZONE", session.LogicalTemporal, kindTimestampUTC)
	case "IntervalYM_DTY", "IntervalYM":
		set("INTERVAL YEAR TO MONTH", session.LogicalTemporal, kindInterval)
	case "IntervalDS_DTY", "IntervalDS":
		set("INTERVAL DAY TO SECOND", session.LogicalTemporal, kindInterval)
	case "RAW":
		set("RAW", session.LogicalBinary, kindBinary)
	case "LongVarChar", "LONG", "OCIClobLocator", "LongRaw", "LONG RAW", "OCIBlobLocator", "OCIFileLocator":
		return col, session.Fail(session.CodeUnsupportedColumn, "column "+quoteColumn(name)+
			" is a LOB, LONG, or JSON value, which remote queries do not read; select a bounded part with DBMS_LOB.SUBSTR, or JSON_SERIALIZE(... RETURNING VARCHAR2)")
	default:
		return col, session.Fail(session.CodeUnsupportedColumn, "column "+quoteColumn(name)+" has a type remote queries do not support; convert it with TO_CHAR or CAST")
	}
	return col, nil
}

func quoteColumn(name string) string {
	if c, ok := session.StringCell(name); ok {
		return c.Token
	}
	return "(unnamed)"
}

// numberTypeName formats a NUMBER column's type with its precision and scale when the
// column has them: NUMBER(10), NUMBER(5,2), or NUMBER.
func numberTypeName(precision, scale int64, hasScale bool) string {
	switch {
	case !hasScale || precision <= 0 || precision > 38:
		return "NUMBER"
	case scale == 0:
		return "NUMBER(" + strconv.FormatInt(precision, 10) + ")"
	case scale > 0 && scale <= 127:
		return "NUMBER(" + strconv.FormatInt(precision, 10) + "," + strconv.FormatInt(scale, 10) + ")"
	}
	return "NUMBER"
}

var errUnsupportedValue = session.Fail(codeUnsupportedValue, "the result has a value remote queries cannot represent")

// remoteCell encodes one driver value of a column (annex: column types).
func remoteCell(kind remoteKind, value any) (session.Cell, *session.Failure) {
	if value == nil {
		return session.Null, nil
	}
	var cell session.Cell
	ok := false
	switch kind {
	case kindDecimal:
		// Never through a float: only the driver's exact text, or an exact integer.
		switch v := value.(type) {
		case string:
			cell, ok = session.DecimalCell(v)
		case []byte:
			cell, ok = session.DecimalCell(string(v))
		case int64:
			cell, ok = session.DecimalCell(strconv.FormatInt(v, 10))
		}
	case kindFloat:
		switch v := value.(type) {
		case float32:
			cell, ok = session.FloatCell(floatText(float64(v), 32))
		case float64:
			cell, ok = session.FloatCell(floatText(v, 64))
		}
	case kindString, kindInterval:
		switch v := value.(type) {
		case string:
			cell, ok = session.StringCell(v)
		case []byte:
			cell, ok = session.StringCell(string(v))
		}
	case kindDate, kindTimestamp, kindTimestampTZ, kindTimestampUTC:
		if v, isTime := value.(time.Time); isTime {
			cell, ok = timeCell(kind, v)
		}
	case kindBinary:
		if v, isBytes := value.([]byte); isBytes {
			cell, ok = session.StringCell(`\x` + hex.EncodeToString(v))
		}
	}
	if !ok {
		return session.Cell{}, errUnsupportedValue
	}
	return cell, nil
}

func floatText(v float64, bits int) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	return strconv.FormatFloat(v, 'g', -1, bits)
}

// timeCell writes a temporal value in ISO 8601: DATE to the second, TIMESTAMP with its
// fraction, TIMESTAMP WITH TIME ZONE with its offset, and TIMESTAMP WITH LOCAL TIME
// ZONE in UTC. Years outside 1 to 9999 have no such form.
func timeCell(kind remoteKind, t time.Time) (session.Cell, bool) {
	if t.Year() < 1 || t.Year() > 9999 {
		return session.Cell{}, false
	}
	var text string
	switch kind {
	case kindDate:
		text = t.Format("2006-01-02T15:04:05")
	case kindTimestamp:
		text = t.Format("2006-01-02T15:04:05.999999999")
	case kindTimestampTZ:
		text = t.Format("2006-01-02T15:04:05.999999999-07:00")
	default:
		text = t.UTC().Format("2006-01-02T15:04:05.999999999Z")
	}
	return session.StringCell(strings.TrimSpace(text))
}
