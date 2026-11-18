// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && test

package logondurationimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logonduration"
)

func clockResetFixture() (time.Time, logonduration.LoginTimestamps) {
	return time.Date(2026, 10, 14, 13, 17, 9, 0, time.UTC), logonduration.LoginTimestamps{
		LoginWindowTime:  time.Date(1976, 3, 31, 15, 0, 2, 824000000, time.UTC),
		LoginTime:        time.Date(1976, 3, 31, 15, 0, 11, 896000000, time.UTC),
		DesktopReadyTime: time.Date(2026, 10, 10, 13, 42, 36, 0, time.UTC),
		FileVaultEnabled: true,
	}
}

func TestValidateTimestamps_ClockResetPayload(t *testing.T) {
	boot, ts := clockResetFixture()
	got, dropped := validateTimestamps(boot, boot.Add(time.Minute), ts)
	assert.Equal(t, logonduration.LoginTimestamps{FileVaultEnabled: true}, got)
	assert.Equal(t, []string{"login_window_ready", "login", "desktop_ready"}, dropped)
}

func TestValidateTimestamps_DropsBeforeBoot(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime: boot.Add(-6 * time.Second),
		LoginTime:       boot.Add(10 * time.Second),
	}
	got, dropped := validateTimestamps(boot, boot.Add(time.Minute), ts)
	assert.Zero(t, got.LoginWindowTime)
	assert.Equal(t, ts.LoginTime, got.LoginTime)
	assert.Equal(t, []string{"login_window_ready"}, dropped)
}

func TestValidateTimestamps_DropsFuture(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	now := boot.Add(time.Minute)
	ts := logonduration.LoginTimestamps{DesktopReadyTime: now.Add(6 * time.Second)}
	got, dropped := validateTimestamps(boot, now, ts)
	assert.Zero(t, got.DesktopReadyTime)
	assert.Equal(t, []string{"desktop_ready"}, dropped)
}

func TestValidateTimestamps_KeepsWithinSkew(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	now := boot.Add(time.Minute)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(-4 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: now.Add(4 * time.Second),
	}
	got, dropped := validateTimestamps(boot, now, ts)
	assert.Equal(t, ts, got)
	assert.Empty(t, dropped)

	// Both bounds include the exact tolerance, as does milestone ordering.
	ts.LoginWindowTime = boot.Add(-clockSkewTolerance)
	ts.DesktopReadyTime = now.Add(clockSkewTolerance)
	got, dropped = validateTimestamps(boot, now, ts)
	assert.Equal(t, ts, got)
	assert.Empty(t, dropped)
	ts.DesktopReadyTime = ts.LoginTime.Add(-clockSkewTolerance)
	got, dropped = validateTimestamps(boot, now, ts)
	assert.Equal(t, ts, got)
	assert.Empty(t, dropped)
}

func TestValidateTimestamps_DropsOutOfOrder(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		ts      logonduration.LoginTimestamps
		want    logonduration.LoginTimestamps
		dropped []string
	}{
		{
			name: "desktop before login",
			ts: logonduration.LoginTimestamps{
				LoginWindowTime: boot.Add(10 * time.Second), LoginTime: boot.Add(90 * time.Second), DesktopReadyTime: boot.Add(30 * time.Second),
			},
			want:    logonduration.LoginTimestamps{LoginWindowTime: boot.Add(10 * time.Second), LoginTime: boot.Add(90 * time.Second)},
			dropped: []string{"desktop_ready"},
		},
		{
			name: "login before window",
			ts: logonduration.LoginTimestamps{
				LoginWindowTime: boot.Add(90 * time.Second), LoginTime: boot.Add(30 * time.Second), DesktopReadyTime: boot.Add(120 * time.Second),
			},
			want:    logonduration.LoginTimestamps{LoginWindowTime: boot.Add(90 * time.Second), DesktopReadyTime: boot.Add(120 * time.Second)},
			dropped: []string{"login"},
		},
		{
			name: "missing login still compares desktop to window",
			ts: logonduration.LoginTimestamps{
				LoginWindowTime: boot.Add(90 * time.Second), DesktopReadyTime: boot.Add(30 * time.Second),
			},
			want:    logonduration.LoginTimestamps{LoginWindowTime: boot.Add(90 * time.Second)},
			dropped: []string{"desktop_ready"},
		},
		{
			name: "mixed bounds and order preserve dropped ID order",
			ts: logonduration.LoginTimestamps{
				LoginWindowTime: boot.Add(90 * time.Second), LoginTime: boot.Add(30 * time.Second), DesktopReadyTime: boot.Add(-time.Minute),
			},
			want:    logonduration.LoginTimestamps{LoginWindowTime: boot.Add(90 * time.Second)},
			dropped: []string{"login", "desktop_ready"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, dropped := validateTimestamps(boot, boot.Add(3*time.Minute), tc.ts)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.dropped, dropped)
		})
	}
}

func TestValidateTimestamps_ValidUnchanged(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime: boot.Add(10 * time.Second), LoginTime: boot.Add(30 * time.Second), DesktopReadyTime: boot.Add(90 * time.Second), FileVaultEnabled: true,
	}
	got, dropped := validateTimestamps(boot, boot.Add(2*time.Minute), ts)
	assert.Equal(t, ts, got)
	assert.Empty(t, dropped)
}

func TestValidateTimestamps_ZeroBootSkipsOnlyLowerBound(t *testing.T) {
	now := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{LoginWindowTime: now.Add(-24 * time.Hour), DesktopReadyTime: now.Add(time.Minute)}
	got, dropped := validateTimestamps(time.Time{}, now, ts)
	assert.Equal(t, ts.LoginWindowTime, got.LoginWindowTime)
	assert.Zero(t, got.DesktopReadyTime)
	assert.Equal(t, []string{"desktop_ready"}, dropped)
}
