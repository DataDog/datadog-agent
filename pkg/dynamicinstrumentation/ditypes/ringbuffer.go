// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ditypes

// DIEvent represents a single invocation of a function and it's captured information
type DIEvent struct {
	ProbeID  string
	PID      uint32
	UID      uint32
	Argdata  []*Param
	StackPCs []byte
}

// Param is the representation of a single function parameter after being parsed from
// the raw byte buffer sent from bpf
type Param struct {
	ValueStr string `json:",omitempty"`
	Type     string
	Size     uint16
	Kind     byte
	Fields   []*Param `json:",omitempty"`
}

// StackFrame represents a single entry in a stack trace
type StackFrame struct {
	FileName string `json:"fileName,omitempty"`
	Function string `json:"function,omitempty"`
	Line     int    `json:"lineNumber,omitempty"`
}

// EventCallback is the function that is called everytime a new event is created
type EventCallback func(*DIEvent)
