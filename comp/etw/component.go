//go:build windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
// Package etw provides ETW tracing facilities to other components
package etw

import (
	"github.com/DataDog/datadog-agent/comp/etw/native"
	"golang.org/x/sys/windows"
)

// team: windows-agent

// EventCallback is a function that will be called when an ETW event is received
type EventCallback func(e *native.DDEtwEvent)

// TraceLevel A value that indicates the maximum level of events that you want the provider to write.
// The provider typically writes an event if the event's level is less than or equal to this value,
// in addition to meeting the MatchAnyKeyword and MatchAllKeyword criteria.
// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
type TraceLevel uint8

//nolint:golint,stylecheck // Keep the Microsoft naming-style
const (
	TRACE_LEVEL_CRITICAL    = TraceLevel(1)
	TRACE_LEVEL_ERROR       = TraceLevel(2)
	TRACE_LEVEL_WARNING     = TraceLevel(3)
	TRACE_LEVEL_INFORMATION = TraceLevel(4)
	TRACE_LEVEL_VERBOSE     = TraceLevel(5)
)

type ProviderConfiguration struct {
	// TraceLevel is a value that indicates the maximum level of events that you want the provider to write.
	// The provider typically writes an event if the event's level is less than or equal to this value.
	// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
	TraceLevel TraceLevel

	// MatchAnyKeyword is a 64-bit bitmask of keywords that determine the categories of events that you want the
	// provider to write
	// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
	MatchAnyKeyword uint64

	// MatchAllKeyword is a 64-bit bitmask of keywords that restricts the events that you want the provider to write.
	// See https://learn.microsoft.com/en-us/windows/win32/api/evntrace/nf-evntrace-enabletraceex2
	MatchAllKeyword uint64

	// PIDs allow filtering by PIDs if non-empty
	PIDs []uint64
}

type ProviderConfigurationFunc func(cfg *ProviderConfiguration)

// Session represents an ETW session. A session can have multiple tracing providers enabled.
type Session interface {
	// ConfigureProvider configures a particular ETW provider identified by its GUID for this session.
	ConfigureProvider(providerGUID windows.GUID, configurations ...ProviderConfigurationFunc) error

	StartTracing(providerGUID windows.GUID, callback EventCallback) error

	// StopTracing stops all tracing activities.
	// It's not possible to use the session anymore after a call to StopTracing.
	StopTracing() error
}

// Component offers a way to create ETW tracing sessions with a given name.
type Component interface {
	NewSession(sessionName string) (Session, error)
}
