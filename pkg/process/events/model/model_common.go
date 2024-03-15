// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -tests=false

//nolint:revive // TODO(PROC) Fix revive linter
package model

import (
	"time"
)

// EventType represents the type of the process lifecycle event
type EventType int32

const (
	// Fork represents a process fork event
	Fork EventType = iota
	// Exec represents a process exec event
	Exec
	// Exit represents a process exit event
	Exit
)

// String returns the string representation of an EventType
func (e EventType) String() string {
	switch e {
	case Fork:
		return "fork"
	case Exec:
		return "exec"
	case Exit:
		return "exit"
	}
	return "unknown"
}

// NewEventType returns the EventType associated with a string
func NewEventType(eventType string) EventType {
	switch eventType {
	case Fork.String():
		return Fork
	case Exec.String():
		return Exec
	case Exit.String():
		return Exit
	}
	return -1
}

// ProcessEvent is a common interface for collected process events shared across multiple event listener implementations
type ProcessEvent struct {
	EventType      EventType `json:"event_type" msg:"event_type"`
	CollectionTime time.Time `json:"collection_time" msg:"collection_time"`
	Pid            uint32    `json:"pid" msg:"pid"`
	ContainerID    string    `json:"container_id" msg:"container_id"`
	Ppid           uint32    `json:"ppid" msg:"ppid"`
	UID            uint32    `json:"uid" msg:"uid"`
	GID            uint32    `json:"gid" msg:"gid"`
	Username       string    `json:"username" msg:"username"`
	Group          string    `json:"group" msg:"group"`
	Exe            string    `json:"exe" msg:"exe"`
	Cmdline        []string  `json:"cmdline" msg:"cmdline"`
	ForkTime       time.Time `json:"fork_time,omitempty" msg:"fork_time,omitempty"`
	ExecTime       time.Time `json:"exec_time,omitempty" msg:"exec_time,omitempty"`
	ExitTime       time.Time `json:"exit_time,omitempty" msg:"exit_time,omitempty"`
	ExitCode       uint32    `json:"exit_code,omitempty" msg:"exit_code,omitempty"`
}

// NewMockedForkEvent creates a mocked Fork event for tests
func NewMockedForkEvent(ts time.Time, pid uint32, exe string, args []string) *ProcessEvent {
	return &ProcessEvent{
		EventType:      Fork,
		CollectionTime: time.Now(),
		Pid:            pid,
		ContainerID:    "01234567890abcedf",
		Ppid:           1,
		UID:            100,
		GID:            100,
		Username:       "dog",
		Group:          "dd-agent",
		Exe:            exe,
		Cmdline:        args,
		ForkTime:       ts,
	}
}

// NewMockedExecEvent creates a mocked Exec event for tests
func NewMockedExecEvent(ts time.Time, pid uint32, exe string, args []string) *ProcessEvent {
	return &ProcessEvent{
		EventType:      Exec,
		CollectionTime: time.Now(),
		Pid:            pid,
		ContainerID:    "01234567890abcedf",
		Ppid:           1,
		UID:            100,
		GID:            100,
		Username:       "dog",
		Group:          "dd-agent",
		Exe:            exe,
		Cmdline:        args,
		ForkTime:       ts,
		ExecTime:       ts,
	}
}

// NewMockedExitEvent creates a mocked Exit event for tests
func NewMockedExitEvent(ts time.Time, pid uint32, exe string, args []string, code uint32) *ProcessEvent {
	return &ProcessEvent{
		EventType:      Exit,
		CollectionTime: time.Now(),
		Pid:            pid,
		ContainerID:    "01234567890abcedf",
		Ppid:           1,
		UID:            100,
		GID:            100,
		Username:       "dog",
		Group:          "dd-agent",
		Exe:            exe,
		Cmdline:        args,
		ForkTime:       ts.Add(-10 * time.Second),
		ExecTime:       ts.Add(-10 * time.Second),
		ExitTime:       ts,
		ExitCode:       code,
	}
}
