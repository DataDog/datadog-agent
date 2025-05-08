// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package redis

type CommandType uint8

var (
	UnknownCommand = CommandType(0x0)
	GetCommand     = CommandType(0x1)
	SetCommand     = CommandType(0x2)
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
