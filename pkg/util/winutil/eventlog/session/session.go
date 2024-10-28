// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package evtsession provides helpers for managing an Event Log API session
// https://learn.microsoft.com/en-us/windows/win32/wes/accessing-remote-computers
package evtsession

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// Session defines the interface for connecting to an Event Log host and is used to
// manage event logs and query, subscribe, and render events.
//
// The Event Log API manages the connection behind the session handle. The connection to the
// remote host is only made when the session is first used (e.g. EvtSubscribe).
// If the remote host or the remote EventLog service is restarted the session handle
// does not need to be closed and re-created.
//
// https://learn.microsoft.com/en-us/windows/win32/wes/accessing-remote-computers
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtopensession
type Session interface {
	// Close closes the session with the target host
	//
	// Close will automatically close any open handles created in the session,
	// so you must not use any subscription or event record handles after closing the session.
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtopensession
	// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	Close()

	// Handle returns the underlying handle returned by EvtOpenSession
	Handle() evtapi.EventSessionHandle
}

type session struct {
	// Windows API
	eventLogAPI evtapi.API
	handle      evtapi.EventSessionHandle
}

func newSession(api evtapi.API) *session {
	var s session
	s.eventLogAPI = api
	s.handle = evtapi.EventSessionHandle(0)

	return &s
}

// New creates a new session to the local event log service
func New(api evtapi.API) Session {
	return newSession(api)
}

// NewRemote creates a new session to a remote event log service
func NewRemote(
	api evtapi.API,
	server string,
	user string,
	domain string,
	password string,
	flags uint,
) (Session, error) {
	s := newSession(api)

	handle, err := s.eventLogAPI.EvtOpenSession(
		server,
		user,
		domain,
		password,
		flags,
	)
	if err != nil {
		return nil, err
	}
	s.handle = handle

	return s, nil
}

func (s *session) Close() {
	if s.handle != evtapi.EventSessionHandle(0) {
		evtapi.EvtCloseSession(s.eventLogAPI, s.handle)
		s.handle = evtapi.EventSessionHandle(0)
	}
}

func (s *session) Handle() evtapi.EventSessionHandle {
	return s.handle
}
