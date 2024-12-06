// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"encoding/json"
	"fmt"
	"regexp"
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

// AssertUnitProperty asserts that the given systemd unit has the given property
func (h *Host) AssertUnitProperty(unit, property, value string) {
	res, err := h.remote.Execute(fmt.Sprintf("sudo systemctl show -p %s %s", property, unit))
	require.NoError(h.t, err)
	require.Equal(h.t, fmt.Sprintf("%s=%s\n", property, value), res)
}

func popIfMatches(searchedEvents []SystemdEvent, log journaldLog) []SystemdEvent {
	for i, event := range searchedEvents {
		if eventMatches(log, event) {
			newEvents := make([]SystemdEvent, 0, len(searchedEvents)-1)
			newEvents = append(newEvents, searchedEvents[:i]...)
			newEvents = append(newEvents, searchedEvents[i+1:]...)
			return newEvents
		}
	}
	return searchedEvents
}

func eventMatches(log journaldLog, event SystemdEvent) bool {
	if log.Unit != event.Unit {
		return false
	}
	match, _ := regexp.MatchString(event.Pattern, log.Message)
	return match
}

// stripSystemd244 removes the events that are not present for systemd versions
// before 244: 'ConditionPathExists' is added in 244 and used by
// system-probe and security-agent
func (h *Host) stripSystemd244(events []SystemdEvent) []SystemdEvent {
	if h.systemdVersion >= 244 {
		return events
	}
	newEvents := make([]SystemdEvent, 0, len(events))
	map244Units := map[string]struct{}{
		"datadog-agent-sysprobe.service":     {},
		"datadog-agent-security.service":     {},
		"datadog-agent-sysprobe-exp.service": {},
		"datadog-agent-security-exp.service": {},
	}
	for _, e := range events {
		if _, ok := map244Units[e.Unit]; ok {
			continue
		}
		newEvents = append(newEvents, e)
	}
	return newEvents
}

// AssertSystemdEvents asserts that the systemd events have been logged since the given timestamp
func (h *Host) AssertSystemdEvents(since JournaldTimestamp, events SystemdEventSequence) {
	var lastSearchedEvents []SystemdEvent
	success := assert.Eventually(h.t, func() bool {
		logs := h.journaldLogsSince(since)
		if len(logs) < len(events.Events) {
			return false
		}
		i, j := 0, 0
		var searchedEvents []SystemdEvent
		for i < len(logs) && j < len(events.Events) {
			if len(searchedEvents) == 0 {
				searchedEvents = events.Events[j]
				j++
			}
			searchedEvents = h.stripSystemd244(searchedEvents)
			searchedEvents = popIfMatches(searchedEvents, logs[i])
			i++
		}
		lastSearchedEvents = searchedEvents
		return j == len(events.Events)
	}, 60*time.Second, 1*time.Second)

	if !success {
		logs := h.journaldLogsSince(since)
		h.t.Logf("Blocked on validating: %v", lastSearchedEvents)
		h.t.Logf("Expected events: %v", events.Events)
		h.t.Logf("Actual events: %v", logs)
	}
}

// SystemdEvent represents a systemd event
type SystemdEvent struct {
	Unit    string
	Pattern string
}

// SystemdEventSequence represents a sequence of systemd events
type SystemdEventSequence struct {
	Events [][]SystemdEvent
}

// SystemdEvents returns a new SystemdEventSequence
func SystemdEvents() SystemdEventSequence {
	return SystemdEventSequence{}
}

// Starting adds a "Starting" event to the sequence
func (s SystemdEventSequence) Starting(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: "Starting.*"}})
	return s
}

// Started adds a "Started" event to the sequence
func (s SystemdEventSequence) Started(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: "Started.*"}})
	return s
}

// Stopping adds a "Stopping" event to the sequence
func (s SystemdEventSequence) Stopping(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: "Stopping.*"}})
	return s
}

// Stopped adds a "Stopped" event to the sequence
func (s SystemdEventSequence) Stopped(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: "Stopped.*"}})
	return s
}

// Failed adds a "Failed" event to the sequence
func (s SystemdEventSequence) Failed(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: "Failed.*"}})
	return s
}

// Timed adds a "Timed" event to the sequence
func (s SystemdEventSequence) Timed(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: "Timed.*"}})
	return s
}

// Skipped adds a "Skipped" event to the sequence
func (s SystemdEventSequence) Skipped(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: ".*skipped.*"}})
	return s
}

// SkippedIf adds a "Skipped" event to the sequence if the condition is true
func (s SystemdEventSequence) SkippedIf(unit string, condition bool) SystemdEventSequence {
	if !condition {
		return s
	}

	return s.Skipped(unit)
}

// SigtermTimed adds a "SigtermTimed" event to the sequence
func (s SystemdEventSequence) SigtermTimed(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: ".*stop-sigterm.*timed out.*"}})
	return s
}

// Sigkill adds a "Sigkill" event to the sequence
func (s SystemdEventSequence) Sigkill(unit string) SystemdEventSequence {
	s.Events = append(s.Events, []SystemdEvent{{Unit: unit, Pattern: ".*status=9/KILL.*"}})
	return s
}

// Unordered adds an unordered sequence of events to the sequence
func (s SystemdEventSequence) Unordered(events SystemdEventSequence) SystemdEventSequence {
	flatten := []SystemdEvent{}
	for _, e := range events.Events {
		flatten = append(flatten, e...)
	}
	s.Events = append(s.Events, flatten)
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
