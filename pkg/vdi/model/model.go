// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package model contains the provider-neutral desktop session inventory exchanged
// between system-probe and VDI core checks.
package model

import "time"

const (
	// ProviderAWSWorkSpaces identifies the Amazon WorkSpaces inventory provider.
	ProviderAWSWorkSpaces = "aws_workspaces"
	// ProtocolDCV identifies Amazon DCV protocol sessions.
	ProtocolDCV = "dcv"

	// StatusOK indicates that a source was collected completely.
	StatusOK = "ok"
	// StatusPartial indicates that a source was collected incompletely.
	StatusPartial = "partial"
	// StatusError indicates that a source could not be collected.
	StatusError = "error"
)

// SourceStatus describes the result of collecting one independent inventory source.
type SourceStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// WindowsSession describes a Windows desktop session discovered through WTS.
type WindowsSession struct {
	WindowsSessionID uint32     `json:"windows_session_id"`
	User             string     `json:"user,omitempty"`
	Domain           string     `json:"domain,omitempty"`
	State            string     `json:"state"`
	LogonAt          *time.Time `json:"logon_at,omitempty"`
	LastInputAt      *time.Time `json:"last_input_at,omitempty"`
}

// Connection describes an optional authenticated connection to a VDI session.
type Connection struct {
	ID                string     `json:"id"`
	AuthenticatedUser string     `json:"authenticated_user,omitempty"`
	Transport         string     `json:"transport,omitempty"`
	ClientMode        string     `json:"client_mode,omitempty"`
	UserAgent         string     `json:"user_agent,omitempty"`
	ConnectedAt       *time.Time `json:"connected_at,omitempty"`
	LastInteractionAt *time.Time `json:"last_interaction_at,omitempty"`
	FirstFrameAt      *time.Time `json:"first_frame_at,omitempty"`
}

// Session describes a provider-scoped VDI session. Connections are optional
// because some providers expose sessions without distinct connection objects.
type Session struct {
	ID               string       `json:"id"`
	Protocol         string       `json:"protocol,omitempty"`
	WindowsSessionID *uint32      `json:"windows_session_id,omitempty"`
	User             string       `json:"user,omitempty"`
	Owner            string       `json:"owner,omitempty"`
	State            string       `json:"state,omitempty"`
	LogonAt          *time.Time   `json:"logon_at,omitempty"`
	LastInputAt      *time.Time   `json:"last_input_at,omitempty"`
	Connections      []Connection `json:"connections,omitempty"`
}

// ProviderInventory contains the sessions returned by one VDI provider.
type ProviderInventory struct {
	SourceStatus
	Sessions []Session `json:"sessions,omitempty"`
}

// InventoryResponse is returned by the local system-probe VDI endpoint.
type InventoryResponse struct {
	CollectedAt time.Time                    `json:"collected_at"`
	Providers   map[string]ProviderInventory `json:"providers"`
}
