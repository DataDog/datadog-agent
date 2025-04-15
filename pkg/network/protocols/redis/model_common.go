// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package redis

// CommandType defines supported redis commands.
type CommandType uint8

var (
	// UnknownCommand is the default CommandType value.
	UnknownCommand = CommandType(0x0)
	// GetCommand represents a GET redis command.
	GetCommand = CommandType(0x1)
	// SetCommand represents a SET redis command.
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
