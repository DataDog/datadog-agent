// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	EventType      EventType `json:"event_type"`
	CollectionTime time.Time `json:"collection_time"`
	Pid            uint32    `json:"pid"`
	ContainerID    string    `json:"container_id"`
	Ppid           uint32    `json:"ppid"`
	UID            uint32    `json:"uid"`
	GID            uint32    `json:"gid"`
	Username       string    `json:"username"`
	Group          string    `json:"group"`
	Exe            string    `json:"exe"`
	Cmdline        []string  `json:"cmdline"`
	ForkTime       time.Time `json:"fork_time,omitempty"`
	ExecTime       time.Time `json:"exec_time,omitempty"`
	ExitTime       time.Time `json:"exit_time,omitempty"`
	ExitCode       uint32    `json:"exit_code,omitempty"`
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

// AssertProcessEvents compares two ProcessEvents. Two events can't be compared using directly assert.Equal
// due to the embedded time fields
func AssertProcessEvents(t *testing.T, expected, actual *ProcessEvent) {
	t.Helper()

	assert.Equal(t, expected.EventType, actual.EventType)
	assert.WithinDuration(t, expected.CollectionTime, actual.CollectionTime, 0)
	assert.Equal(t, expected.Pid, actual.Pid)
	assert.Equal(t, expected.ContainerID, actual.ContainerID)
	assert.Equal(t, expected.Ppid, actual.Ppid)
	assert.Equal(t, expected.UID, actual.UID)
	assert.Equal(t, expected.GID, actual.GID)
	assert.Equal(t, expected.Username, actual.Username)
	assert.Equal(t, expected.Group, actual.Group)
	assert.Equal(t, expected.Exe, actual.Exe)
	assert.Equal(t, expected.Cmdline, actual.Cmdline)
	assert.WithinDuration(t, expected.ForkTime, actual.ForkTime, 0)
	assert.WithinDuration(t, expected.ExecTime, actual.ExecTime, 0)
	assert.WithinDuration(t, expected.ExitTime, actual.ExitTime, 0)
	assert.Equal(t, expected.ExitCode, actual.ExitCode)
}
