// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package windows provides reusable Windows desktop-session inventory primitives.
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
	wtsUserName   = 5
	wtsDomainName = 7
	wtsInfo       = 24
)

var (
	wtsapi32                       = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSEnumerateSessionsW      = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInformation = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory              = wtsapi32.NewProc("WTSFreeMemory")
	procProcessIDToSessionID       = windows.NewLazySystemDLL("kernel32.dll").NewProc("ProcessIdToSessionId")
)

type wtsSessionInfo struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

type wtsInfoW struct {
	State              uint32
	SessionID          uint32
	IncomingBytes      uint32
	OutgoingBytes      uint32
	IncomingFrames     uint32
	OutgoingFrames     uint32
	IncomingCompressed uint32
	OutgoingCompressed uint32
	WinStationName     [33]uint16
	Domain             [18]uint16
	UserName           [21]uint16
	ConnectTime        int64
	DisconnectTime     int64
	LastInputTime      int64
	LogonTime          int64
	CurrentTime        int64
}

// EnumerateSessions returns the current local Windows desktop sessions.
func EnumerateSessions() ([]vdimodel.WindowsSession, error) {
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
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer))) }()

	raw := unsafe.Slice(buffer, count)
	sessions := make([]vdimodel.WindowsSession, 0, count)
	for _, item := range raw {
		// WTS can enumerate listener and system sessions whose optional user
		// fields are unavailable. Preserve the session instead of failing the
		// entire inventory.
		user, _ := queryString(item.SessionID, wtsUserName)
		domain, _ := queryString(item.SessionID, wtsDomainName)

		session := vdimodel.WindowsSession{
			WindowsSessionID: item.SessionID,
			User:             user,
			Domain:           domain,
			State:            stateName(item.State),
		}
		if info, err := queryInfo(item.SessionID); err == nil {
			session.LogonAt = windowsTimestamp(info.LogonTime)
			session.LastInputAt = windowsTimestamp(info.LastInputTime)
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// SessionIDForProcess resolves a process to its Windows desktop session.
func SessionIDForProcess(processID uint32) (uint32, error) {
	var sessionID uint32
	success, _, callErr := procProcessIDToSessionID.Call(
		uintptr(processID),
		uintptr(unsafe.Pointer(&sessionID)),
	)
	if success == 0 {
		return 0, fmt.Errorf("ProcessIdToSessionId(%d) failed: %w", processID, normalizeCallError(callErr))
	}
	return sessionID, nil
}

func queryString(sessionID uint32, infoClass uint32) (string, error) {
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
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer))) }()
	if buffer == nil || bytesReturned < 2 {
		return "", nil
	}
	return windows.UTF16PtrToString(buffer), nil
}

func queryInfo(sessionID uint32) (*wtsInfoW, error) {
	var buffer *wtsInfoW
	var bytesReturned uint32
	success, _, callErr := procWTSQuerySessionInformation.Call(
		0,
		uintptr(sessionID),
		uintptr(wtsInfo),
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if success == 0 {
		return nil, normalizeCallError(callErr)
	}
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer))) }()
	if buffer == nil || bytesReturned < uint32(unsafe.Sizeof(wtsInfoW{})) {
		return nil, errors.New("WTSInfo response was truncated")
	}
	copy := *buffer
	return &copy, nil
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
