// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package windows

import (
	"errors"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
)

type fakeWTS struct {
	sessions     []wtsSessionInfo
	enumerateErr error
	strings      map[uint32]map[uint32]string
	info         map[uint32]wtsInfo
}

func (f fakeWTS) enumerateSessions() ([]wtsSessionInfo, error) {
	return f.sessions, f.enumerateErr
}

func (f fakeWTS) queryString(sessionID uint32, infoClass uint32) (string, error) {
	value, ok := f.strings[sessionID][infoClass]
	if !ok {
		return "", errors.New("information unavailable")
	}
	return value, nil
}

func (f fakeWTS) queryInfo(sessionID uint32) (wtsInfo, error) {
	value, ok := f.info[sessionID]
	if !ok {
		return wtsInfo{}, errors.New("information unavailable")
	}
	return value, nil
}

func TestEnumerateSessions(t *testing.T) {
	windowsEpoch := int64(116444736000000000)
	sessions, err := enumerateSessions(fakeWTS{
		sessions: []wtsSessionInfo{{SessionID: 4, State: 0}, {SessionID: 5, State: 4}},
		strings: map[uint32]map[uint32]string{
			4: {wtsUserName: "alice", wtsDomainName: "EXAMPLE"},
		},
		info: map[uint32]wtsInfo{
			4: {
				LogonTime:     windowsEpoch + int64(5*time.Second/(100*time.Nanosecond)),
				LastInputTime: windowsEpoch + int64(10*time.Second/(100*time.Nanosecond)),
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, uint32(4), sessions[0].WindowsSessionID)
	require.Equal(t, "alice", sessions[0].User)
	require.Equal(t, "EXAMPLE", sessions[0].Domain)
	require.Equal(t, "active", sessions[0].State)
	require.Equal(t, time.Unix(5, 0).UTC(), *sessions[0].LogonAt)
	require.Equal(t, time.Unix(10, 0).UTC(), *sessions[0].LastInputAt)
	require.Equal(t, uint32(5), sessions[1].WindowsSessionID)
	require.Equal(t, "disconnected", sessions[1].State)
	require.Empty(t, sessions[1].User)
	require.Nil(t, sessions[1].LogonAt)
}

func TestEnumerateSessionsReturnsEnumerationError(t *testing.T) {
	_, err := enumerateSessions(fakeWTS{enumerateErr: errors.New("enumeration failed")})
	require.EqualError(t, err, "enumeration failed")
}

func TestStateName(t *testing.T) {
	require.Equal(t, "active", stateName(0))
	require.Equal(t, "disconnected", stateName(4))
	require.Equal(t, "unknown_99", stateName(99))
}

func TestWindowsTimestamp(t *testing.T) {
	require.Nil(t, windowsTimestamp(0))
	windowsEpoch := int64(116444736000000000)
	result := windowsTimestamp(windowsEpoch + int64(5*time.Second/(100*time.Nanosecond)))
	require.NotNil(t, result)
	require.Equal(t, time.Unix(5, 0).UTC(), *result)
}

func TestWTSInfoLayout(t *testing.T) {
	var info wtsInfo
	require.Equal(t, uintptr(216), unsafe.Sizeof(info))
	require.Equal(t, uintptr(176), unsafe.Offsetof(info.ConnectTime))
}
