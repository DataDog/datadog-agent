// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import "strings"

// Operation represents a postgres query operation supported by our decoder.
type Operation uint8

const (
	// UnknownOP represents an unknown operation.
	UnknownOP Operation = iota
	// SelectOP represents a SELECT operation.
	SelectOP
	// InsertOP represents an INSERT operation.
	InsertOP
	// UpdateOP represents an UPDATE operation.
	UpdateOP
	// CreateTableOP represents a CREATE TABLE operation.
	CreateTableOP
	// DropTableOP represents a DROP TABLE operation.
	DropTableOP
	// DeleteTableOP represents a DELETE TABLE operation.
	DeleteTableOP
	// AlterTableOP represents an ALTER TABLE operation.
	AlterTableOP
	// TruncateTableOP represents a TRUNCATE operation.
	TruncateTableOP
	// ShowOP represents a command SHOW
	ShowOP
)

// String returns the string representation of the operation.
func (op Operation) String() string {
	switch op {
	case SelectOP:
		return "SELECT"
	case InsertOP:
		return "INSERT"
	case UpdateOP:
		return "UPDATE"
	case CreateTableOP:
		return "CREATE"
	case DropTableOP:
		return "DROP"
	case TruncateTableOP:
		return "TRUNCATE"
	case DeleteTableOP:
		return "DELETE"
	case AlterTableOP:
		return "ALTER"
	case ShowOP:
		return "SHOW"
	default:
		return "UNKNOWN"
	}
}

// FromString returns the Operation from a string.
func FromString(op string) Operation {
	switch strings.ToUpper(op) {
	case "SELECT":
		return SelectOP
	case "INSERT":
		return InsertOP
	case "UPDATE":
		return UpdateOP
	case "CREATE":
		return CreateTableOP
	case "DROP":
		return DropTableOP
	case "TRUNCATE":
		return TruncateTableOP
	case "DELETE":
		return DeleteTableOP
	case "ALTER":
		return AlterTableOP
	case "SHOW":
		return ShowOP
	default:
		return UnknownOP
	}
}
