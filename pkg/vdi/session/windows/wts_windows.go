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
	wtsUserName         = 5
	wtsDomainName       = 7
	wtsSessionInfoClass = 24
)

var (
	wtsapi32                       = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSEnumerateSessionsW      = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInformation = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory              = wtsapi32.NewProc("WTSFreeMemory")
)

type wtsSessionInfo struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

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
	enumerateSessions() ([]wtsSessionInfo, error)
	queryString(sessionID uint32, infoClass uint32) (string, error)
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
		session := vdimodel.WindowsSession{
			WindowsSessionID: item.SessionID,
			User:             user,
			Domain:           domain,
			State:            stateName(item.State),
		}
		if info, queryErr := api.queryInfo(item.SessionID); queryErr == nil {
			session.LogonAt = windowsTimestamp(info.LogonTime)
			session.LastInputAt = windowsTimestamp(info.LastInputTime)
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (nativeWTS) enumerateSessions() ([]wtsSessionInfo, error) {
	var buffer *wtsSessionInfo
	var count uint32
	success, _, callErr := procWTSEnumerateSessionsW.Call(
		0,
		0,
		1,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&count)),
	)
	if success == 0 {
		return nil, fmt.Errorf("WTSEnumerateSessionsW failed: %w", normalizeCallError(callErr))
	}
	if count == 0 {
		return nil, nil
	}
	if buffer == nil {
		return nil, errors.New("WTSEnumerateSessionsW returned an empty response")
	}
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer))) }()
	return append([]wtsSessionInfo(nil), unsafe.Slice(buffer, count)...), nil
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
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer))) }()
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
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer))) }()
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
		"active",
		"connected",
		"connect_query",
		"shadow",
		"disconnected",
		"idle",
		"listen",
		"reset",
		"down",
		"init",
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
