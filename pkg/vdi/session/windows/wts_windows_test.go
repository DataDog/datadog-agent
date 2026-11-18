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

	"github.com/DataDog/datadog-agent/pkg/vdi/model"
	"github.com/DataDog/datadog-agent/pkg/vdi/provider/avd"
)

type fakeWTS struct {
	sessions     []wtsSessionInfo
	enumerateErr error
	strings      map[uint32]map[uint32]string
	info         map[uint32]wtsInfo
	protocols    map[uint32]uint16
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
			4: {wtsUserName: "alice", wtsDomainName: "EXAMPLE", wtsWinStationName: "rdp-tcp#4"},
		},
		protocols: map[uint32]uint16{4: 2, 5: 0},
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
	require.Equal(t, "rdp-tcp#4", sessions[0].Name)
	require.Equal(t, "active", sessions[0].State)
	require.Equal(t, "rdp", sessions[0].Protocol)
	require.Equal(t, "console", sessions[1].Protocol)
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

func TestDisconnectedSessionQueryFailuresMakeInventoryPartial(t *testing.T) {
	sessions, err := enumerateSessions(fakeWTS{
		sessions: []wtsSessionInfo{{SessionID: 3, State: 4}},
	})
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	inventory := avd.FromWindowsInventory(model.WindowsInventory{
		SourceStatus: model.SourceStatus{Status: model.StatusOK},
		Sessions:     sessions,
	})
	require.Equal(t, model.StatusPartial, inventory.Status)
	require.Empty(t, inventory.Sessions)
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

func (f fakeWTS) queryProtocol(sessionID uint32) (uint16, error) {
	protocol, ok := f.protocols[sessionID]
	if !ok {
		return 0, errors.New("protocol unavailable")
	}
	return protocol, nil
}
