// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && test

package logonduration

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testTimestampQueries() (timestampQueries, LoginTimestamps) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
		FileVaultEnabled: true,
	}
	return timestampQueries{
		fileVaultEnabled: func() (bool, error) { return true, nil },
		loginWindowTime:  func(bool) (time.Time, error) { return ts.LoginWindowTime, nil },
		loginTime:        func() (time.Time, error) { return ts.LoginTime, nil },
		desktopReadyTime: func() (time.Time, error) { return ts.DesktopReadyTime, nil },
	}, ts
}

func TestCollectLoginTimestamps_FileVaultFallsBackToLoginWindow(t *testing.T) {
	q, expected := testTimestampQueries()
	var calls []bool
	q.loginWindowTime = func(fv bool) (time.Time, error) {
		calls = append(calls, fv)
		if fv {
			return time.Time{}, errors.New("FVUnlock entry missing after authenticated restart")
		}
		return expected.LoginWindowTime, nil
	}
	got := collectLoginTimestamps(q)
	assert.Equal(t, expected, got)
	assert.True(t, got.FileVaultEnabled)
	assert.Equal(t, []bool{true, false}, calls)
}

func TestCollectLoginTimestamps_FileVaultUnlockFound(t *testing.T) {
	q, expected := testTimestampQueries()
	var calls []bool
	q.loginWindowTime = func(fv bool) (time.Time, error) {
		calls = append(calls, fv)
		return expected.LoginWindowTime, nil
	}
	assert.Equal(t, expected, collectLoginTimestamps(q))
	assert.Equal(t, []bool{true}, calls)
}

func TestCollectLoginTimestamps_FileVaultOff(t *testing.T) {
	q, expected := testTimestampQueries()
	q.fileVaultEnabled = func() (bool, error) { return false, nil }
	expected.FileVaultEnabled = false
	var calls []bool
	q.loginWindowTime = func(fv bool) (time.Time, error) {
		calls = append(calls, fv)
		return expected.LoginWindowTime, nil
	}
	assert.Equal(t, expected, collectLoginTimestamps(q))
	assert.Equal(t, []bool{false}, calls)
}

func TestCollectLoginTimestamps_BothLoginWindowQueriesFail(t *testing.T) {
	q, expected := testTimestampQueries()
	var calls []bool
	q.loginWindowTime = func(fv bool) (time.Time, error) {
		calls = append(calls, fv)
		return time.Time{}, errors.New("OSLog entry missing")
	}
	expected.LoginWindowTime = time.Time{}
	assert.Equal(t, expected, collectLoginTimestamps(q))
	assert.Equal(t, []bool{true, false}, calls)
}
