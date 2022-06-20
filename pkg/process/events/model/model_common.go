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

const (
	// Exec TODO <processes>
	Exec = "exec"
	// Fork TODO <processes>
	Fork = "fork"
	// Exit TODO <processes>
	Exit = "exit"
)

// ProcessEvent is a common interface for collected process events shared across multiple event listener implementations
type ProcessEvent struct {
	EventType      string    `json:"event_type"`
	CollectionTime time.Time `json:"collection_time"`
	Pid            uint32    `json:"pid"`
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
}

// NewMockedProcessEvent creates a mocked ProcessEvent for tests
func NewMockedProcessEvent(evtType string, ts time.Time, pid uint32, exe string, args []string) *ProcessEvent {
	var forkTime, execTime, exitTime time.Time
	switch evtType {
	case Fork:
		forkTime = ts
	case Exec:
		execTime = ts
	case Exit:
		exitTime = ts
	}

	return &ProcessEvent{
		EventType:      evtType,
		CollectionTime: time.Now(),
		Pid:            pid,
		Ppid:           1,
		UID:            100,
		GID:            100,
		Username:       "dog",
		Group:          "dd-agent",
		Exe:            exe,
		Cmdline:        args,
		ForkTime:       forkTime,
		ExecTime:       execTime,
		ExitTime:       exitTime,
	}
}

// AssertProcessEvents compares two ProcessEvents. Two events can't be compared using directly assert.Equal
// due to the embedded time fields
func AssertProcessEvents(t *testing.T, expected, actual *ProcessEvent) {
	t.Helper()

	assert.Equal(t, expected.EventType, actual.EventType)
	assert.WithinDuration(t, expected.CollectionTime, actual.CollectionTime, 0)
	assert.Equal(t, expected.Pid, actual.Pid)
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
}
