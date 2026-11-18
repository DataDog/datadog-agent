// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package windows collects local Windows desktop sessions through WTS.
package windows

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
)

const (
	wtsUserName           = 5
	wtsWinStationName     = 6
	wtsDomainName         = 7
	wtsSessionInfoClass   = 24
	wtsClientProtocolType = 16
)

var (
	wtsapi32                       = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInformation = wtsapi32.NewProc("WTSQuerySessionInformationW")
)

type wtsInfo struct {
	State              uint32
	SessionID          uint32
	IncomingBytes      uint32
	OutgoingBytes      uint32
	IncomingFrames     uint32
	OutgoingFrames     uint32
	IncomingCompressed uint32
	OutgoingCompressed uint32
	WinStationName     [32]uint16
	Domain             [17]uint16
	UserName           [21]uint16
	ConnectTime        int64
	DisconnectTime     int64
	LastInputTime      int64
	LogonTime          int64
	CurrentTime        int64
}

type wtsAPI interface {
	enumerateSessions() ([]windows.WTS_SESSION_INFO, error)
	queryString(sessionID uint32, infoClass uint32) (string, error)
	queryProtocol(sessionID uint32) (uint16, error)
	queryInfo(sessionID uint32) (wtsInfo, error)
}

type nativeWTS struct{}

// EnumerateSessions returns the current local Windows desktop sessions.
func EnumerateSessions() ([]vdimodel.WindowsSession, error) {
	return enumerateSessions(nativeWTS{})
}

func enumerateSessions(api wtsAPI) ([]vdimodel.WindowsSession, error) {
	raw, err := api.enumerateSessions()
	if err != nil {
		return nil, err
	}

	sessions := make([]vdimodel.WindowsSession, 0, len(raw))
	for _, item := range raw {
		// Listener and system sessions do not always expose user details. Keep
		// the session in the inventory when an optional query fails.
		user, _ := api.queryString(item.SessionID, wtsUserName)
		domain, _ := api.queryString(item.SessionID, wtsDomainName)
		name, _ := api.queryString(item.SessionID, wtsWinStationName)
		session := vdimodel.WindowsSession{
			Name:             name,
			WindowsSessionID: item.SessionID,
			User:             user,
			Domain:           domain,
			State:            stateName(item.State),
		}
		if info, queryErr := api.queryInfo(item.SessionID); queryErr == nil {
			session.LogonAt = windowsTimestamp(info.LogonTime)
			session.LastInputAt = windowsTimestamp(info.LastInputTime)
		}
		if protocol, protocolErr := api.queryProtocol(item.SessionID); protocolErr == nil {
			switch protocol {
			case 0:
				session.Protocol = "console"
			case 2:
				session.Protocol = vdimodel.ProtocolRDP
			default:
				session.Protocol = fmt.Sprintf("unknown_%d", protocol)
			}
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (nativeWTS) enumerateSessions() ([]windows.WTS_SESSION_INFO, error) {
	var buffer *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &buffer, &count); err != nil {
		return nil, fmt.Errorf("WTSEnumerateSessionsW failed: %w", err)
	}
	if buffer != nil {
		defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	}
	if count == 0 {
		return nil, nil
	}
	if buffer == nil {
		return nil, errors.New("WTSEnumerateSessionsW returned an empty response")
	}
	sessions := make([]windows.WTS_SESSION_INFO, count)
	for i, session := range unsafe.Slice(buffer, count) {
		// The station-name pointer belongs to the WTS allocation. Only retain
		// ID and state; the name is queried separately while enriching sessions.
		sessions[i] = windows.WTS_SESSION_INFO{SessionID: session.SessionID, State: session.State}
	}
	return sessions, nil
}

func (nativeWTS) queryString(sessionID uint32, infoClass uint32) (string, error) {
	var buffer *uint16
	var bytesReturned uint32
	success, _, callErr := procWTSQuerySessionInformation.Call(
		0,
		uintptr(sessionID),
		uintptr(infoClass),
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if success == 0 {
		return "", normalizeCallError(callErr)
	}
	if buffer == nil {
		return "", nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	if bytesReturned < 2 {
		return "", nil
	}
	return windows.UTF16PtrToString(buffer), nil
}

func (nativeWTS) queryInfo(sessionID uint32) (wtsInfo, error) {
	var buffer *wtsInfo
	var bytesReturned uint32
	success, _, callErr := procWTSQuerySessionInformation.Call(
		0,
		uintptr(sessionID),
		wtsSessionInfoClass,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if success == 0 {
		return wtsInfo{}, normalizeCallError(callErr)
	}
	if buffer == nil {
		return wtsInfo{}, errors.New("WTSSessionInfo returned an empty response")
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	if bytesReturned < uint32(unsafe.Sizeof(wtsInfo{})) {
		return wtsInfo{}, errors.New("WTSSessionInfo response was truncated")
	}
	return *buffer, nil
}

func windowsTimestamp(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	const windowsToUnix100ns = int64(116444736000000000)
	unix100ns := value - windowsToUnix100ns
	if unix100ns <= 0 {
		return nil
	}
	timestamp := time.Unix(unix100ns/10_000_000, (unix100ns%10_000_000)*100).UTC()
	return &timestamp
}

func stateName(state uint32) string {
	names := [...]string{
		windows.WTSActive:       "active",
		windows.WTSConnected:    "connected",
		windows.WTSConnectQuery: "connect_query",
		windows.WTSShadow:       "shadow",
		windows.WTSDisconnected: "disconnected",
		windows.WTSIdle:         "idle",
		windows.WTSListen:       "listen",
		windows.WTSReset:        "reset",
		windows.WTSDown:         "down",
		windows.WTSInit:         "init",
	}
	if int(state) < len(names) {
		return names[state]
	}
	return fmt.Sprintf("unknown_%d", state)
}

func normalizeCallError(err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return syscall.EINVAL
	}
	return err
}

func (nativeWTS) queryProtocol(sessionID uint32) (uint16, error) {
	var buffer *uint16
	var bytesReturned uint32
	success, _, callErr := procWTSQuerySessionInformation.Call(
		0, uintptr(sessionID), uintptr(wtsClientProtocolType),
		uintptr(unsafe.Pointer(&buffer)), uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if success == 0 {
		return 0, normalizeCallError(callErr)
	}
	if buffer == nil {
		return 0, errors.New("WTSClientProtocolType returned an empty response")
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	if bytesReturned < 2 {
		return 0, errors.New("WTSClientProtocolType response was truncated")
	}
	return *buffer, nil
}
