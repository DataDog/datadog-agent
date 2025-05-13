// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package redis

// CommandType represents a Redis command type.
type CommandType uint8

var (
	// UnknownCommand represents an unknown Redis command.
	UnknownCommand = CommandType(0x0)
	// GetCommand represents the GET Redis command.
	GetCommand = CommandType(0x1)
	// SetCommand represents the SET Redis command.
	SetCommand = CommandType(0x2)
)

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
