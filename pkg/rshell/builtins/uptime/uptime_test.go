// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Tests are in package uptime (not uptime_test) to access unexported formatting
// functions directly, giving deterministic coverage without any OS syscalls.
package uptime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/sysinfo"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    string
	}{
		{"zero", 0, "0 min"},
		{"30 seconds", 30, "0 min"},
		{"1 minute", 60, "1 min"},
		{"59 minutes", 3540, "59 min"},
		{"1 hour", 3600, " 1:00"},
		{"1 hour 30 min", 5400, " 1:30"},
		{"23 hours 59 min", 86340, "23:59"},
		{"exactly 1 day", 86400, "1 day, 0 min"},
		{"1 day 2 hours 3 min", 93780, "1 day,  2:03"},
		{"2 days", 172800, "2 days, 0 min"},
		{"11 days 16 hours 28 min", 1009680, "11 days, 16:28"},
		{"1 day 5 min (no hours)", 86700, "1 day, 5 min"},
		{"9 hours", 32400, " 9:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatDuration(tt.seconds))
		})
	}
}

func TestFormatPretty(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    string
	}{
		{"zero", 0, "up 0 minutes"},
		{"30 seconds", 30, "up 0 minutes"},
		{"1 minute", 60, "up 1 minute"},
		{"2 minutes", 120, "up 2 minutes"},
		{"59 minutes", 3540, "up 59 minutes"},
		{"1 hour", 3600, "up 1 hour"},
		{"2 hours", 7200, "up 2 hours"},
		{"1 hour 1 minute", 3660, "up 1 hour, 1 minute"},
		{"2 hours 30 minutes", 9000, "up 2 hours, 30 minutes"},
		{"1 day", 86400, "up 1 day"},
		{"2 days", 172800, "up 2 days"},
		{"1 day 1 hour", 90000, "up 1 day, 1 hour"},
		{"1 day 1 hour 1 minute", 90060, "up 1 day, 1 hour, 1 minute"},
		{"1 week", 604800, "up 1 week"},
		{"2 weeks", 1209600, "up 2 weeks"},
		{"1 week 4 days", 950400, "up 1 week, 4 days"},
		{"11 days 16 hours 28 minutes", 1009680, "up 1 week, 4 days, 16 hours, 28 minutes"},
		{"1 year", 31536000, "up 1 year"},
		{"1 year 2 weeks", 32745600, "up 1 year, 2 weeks"},
		{"1 decade", 315360000, "up 1 decade"},
		{"1 decade 5 years", 473040000, "up 1 decade, 5 years"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatPretty(tt.seconds))
		})
	}
}

func TestFormatDefault(t *testing.T) {
	now := time.Date(2026, 7, 21, 15, 36, 9, 0, time.UTC)

	t.Run("with load average", func(t *testing.T) {
		info := sysinfo.Info{
			UptimeSeconds: 1009680, // 11 days 16:28
			Load1:         0.23,
			Load5:         0.17,
			Load15:        0.11,
			LoadAvailable: true,
		}
		got := formatDefault(now, info)
		assert.Equal(t, " 15:36:09 up 11 days, 16:28,  load average: 0.23, 0.17, 0.11", got)
	})

	t.Run("without load average", func(t *testing.T) {
		info := sysinfo.Info{
			UptimeSeconds: 5400, // 1:30
			LoadAvailable: false,
		}
		got := formatDefault(now, info)
		assert.Equal(t, " 15:36:09 up  1:30", got)
	})
}
