// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater contains tests for the updater package
package updater

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getMonotonicTimestamp(t *testing.T, host *components.RemoteHost) int64 {
	res := strings.TrimSpace(host.MustExecute("journalctl -n 1 --output=json"))
	var log JournaldLog
	err := json.Unmarshal([]byte(res), &log)
	require.NoError(t, err)
	return log.MonotonicTimestamp
}

// JournaldLog represents a log entry from journald
type JournaldLog struct {
	Unit               string `json:"UNIT"`
	Message            string `json:"MESSAGE"`
	MonotonicTimestamp int64  `json:"__MONOTONIC_TIMESTAMP,string"`
}

func stopCondition(requiredLogs []JournaldLog) func([]JournaldLog) bool {
	return func(logs []JournaldLog) bool {
	L1:
		for _, requiredLog := range requiredLogs {
			for _, log := range logs {
				if log.Unit == requiredLog.Unit && strings.HasPrefix(log.Message, requiredLog.Message) {
					continue L1
				}
			}
			return false
		}
		return true
	}
}

func verifyLogs(received, expected []JournaldLog) bool {
	i, j := 0, 0
	for i < len(received) && j < len(expected) {
		if received[i].Unit == expected[j].Unit && strings.HasPrefix(received[i].Message, expected[j].Message) {
			j++
		}
		i++
	}
	return j == len(expected)
}

func getJournalDOnCondition(t *testing.T, host *components.RemoteHost, minTimestamp int64, condition func([]JournaldLog) bool) []JournaldLog {
	var logs []JournaldLog
	assert.Eventually(t,
		func() bool {
			logs = getOrderedJournaldLogs(t, host, minTimestamp)
			return condition(logs)
		}, 10*time.Second, 100*time.Millisecond)
	return logs
}

func getOrderedJournaldLogs(t *testing.T, host *components.RemoteHost, minTimestamp int64) []JournaldLog {
	host.MustExecute(`journalctl --output=json _COMM=systemd -u datadog* > /tmp/journald_logs`)
	file, err := host.ReadFile("/tmp/journald_logs")
	require.NoError(t, err)
	lines := strings.Split(string(file), "\n")
	logs := make([]JournaldLog, 0, len(lines))
	for _, line := range lines {
		var log JournaldLog
		_ = json.Unmarshal([]byte(line), &log)
		if log.MonotonicTimestamp > minTimestamp {
			logs = append(logs, log)
		}
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].MonotonicTimestamp < logs[j].MonotonicTimestamp
	})
	return logs
}
