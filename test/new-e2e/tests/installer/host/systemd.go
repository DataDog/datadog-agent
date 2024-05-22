// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type journaldLog struct {
	Unit               string `json:"UNIT"`
	Message            string `json:"MESSAGE"`
	MonotonicTimestamp int64  `json:"__MONOTONIC_TIMESTAMP,string"`
}

// JournaldTimestamp represents a monotonic timestamp of a journald log entry
type JournaldTimestamp int64

// LastJournaldTimestamp returns the monotonic timestamp of the last journald log entry
func (h *Host) LastJournaldTimestamp() JournaldTimestamp {
	res := strings.TrimSpace(h.remote.MustExecute("sudo journalctl -n 1 --output=json 2> /dev/null"))
	var log journaldLog
	err := json.Unmarshal([]byte(res), &log)
	require.NoError(h.t, err, res)
	return JournaldTimestamp(log.MonotonicTimestamp)
}

// AssertSystemdEvents asserts that the systemd events have been logged since the given timestamp
func (h *Host) AssertSystemdEvents(since JournaldTimestamp, events SystemdEventSequence) {
	success := assert.Eventually(h.t, func() bool {
		logs := h.journaldLogsSince(since)
		if len(logs) < len(events.Events) {
			return false
		}
		i := 0
		j := 0
		for i < len(logs) && j < len(events.Events) {
			if logs[i].Unit == events.Events[j].Unit && strings.HasPrefix(logs[i].Message, events.Events[j].MessagePrefix) {
				j++
			}
			i++
		}
		return j == len(events.Events)
	}, 30*time.Second, 1*time.Second)
	if !success {
		logs := h.journaldLogsSince(since)
		h.t.Logf("Expected events: %v", events.Events)
		h.t.Logf("Actual events: %v", logs)
	}
}

// SystemdEvent represents a systemd event
type SystemdEvent struct {
	Unit          string
	MessagePrefix string
}

// SystemdEventSequence represents a sequence of systemd events
type SystemdEventSequence struct {
	Events []SystemdEvent
}

// SystemdEvents returns a new SystemdEventSequence
func SystemdEvents() SystemdEventSequence {
	return SystemdEventSequence{}
}

// Starting adds a "Starting" event to the sequence
func (s SystemdEventSequence) Starting(unit string) SystemdEventSequence {
	s.Events = append(s.Events, SystemdEvent{Unit: unit, MessagePrefix: "Starting"})
	return s
}

// Started adds a "Started" event to the sequence
func (s SystemdEventSequence) Started(unit string) SystemdEventSequence {
	s.Events = append(s.Events, SystemdEvent{Unit: unit, MessagePrefix: "Started"})
	return s
}

// Stopping adds a "Stopping" event to the sequence
func (s SystemdEventSequence) Stopping(unit string) SystemdEventSequence {
	s.Events = append(s.Events, SystemdEvent{Unit: unit, MessagePrefix: "Stopping"})
	return s
}

// Stopped adds a "Stopped" event to the sequence
func (s SystemdEventSequence) Stopped(unit string) SystemdEventSequence {
	s.Events = append(s.Events, SystemdEvent{Unit: unit, MessagePrefix: "Stopped"})
	return s
}

// Failed adds a "Failed" event to the sequence
func (s SystemdEventSequence) Failed(unit string) SystemdEventSequence {
	s.Events = append(s.Events, SystemdEvent{Unit: unit, MessagePrefix: "Failed"})
	return s
}

func (h *Host) journaldLogsSince(since JournaldTimestamp) []journaldLog {
	h.remote.MustExecute("sudo journalctl --output=json _COMM=systemd -u datadog* 1> /tmp/journald_logs")
	file, err := h.ReadFile("/tmp/journald_logs")
	require.NoError(h.t, err)
	lines := strings.Split(string(file), "\n")
	logs := make([]journaldLog, 0, len(lines))
	for _, line := range lines {
		var log journaldLog
		_ = json.Unmarshal([]byte(line), &log)
		if log.MonotonicTimestamp > int64(since) {
			logs = append(logs, log)
		}
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].MonotonicTimestamp < logs[j].MonotonicTimestamp
	})
	return logs
}
