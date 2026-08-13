// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
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
	OSSessionID uint32     `json:"os_session_id"`
	OSUser      string     `json:"os_user,omitempty"`
	Domain      string     `json:"domain,omitempty"`
	State       string     `json:"state"`
	LogonAt     *time.Time `json:"logon_at,omitempty"`
	LastInputAt *time.Time `json:"last_input_at,omitempty"`
}

// WindowsInventory contains Windows sessions and their independent source status.
type WindowsInventory struct {
	SourceStatus
	Sessions []WindowsSession `json:"sessions,omitempty"`
}

// DesktopConnection describes an authenticated connection to a protocol session.
type DesktopConnection struct {
	ConnectionID      string     `json:"connection_id"`
	AuthenticatedUser string     `json:"authenticated_user,omitempty"`
	Transport         string     `json:"transport,omitempty"`
	ClientMode        string     `json:"client_mode,omitempty"`
	UserAgent         string     `json:"user_agent,omitempty"`
	ConnectedAt       *time.Time `json:"connected_at,omitempty"`
	LastInteractionAt *time.Time `json:"last_interaction_at,omitempty"`
	FirstFrameAt      *time.Time `json:"first_frame_at,omitempty"`
}

// ProtocolSession describes a provider-specific desktop protocol session.
type ProtocolSession struct {
	Provider          string              `json:"provider"`
	Protocol          string              `json:"protocol"`
	ProtocolSessionID string              `json:"protocol_session_id"`
	Owner             string              `json:"owner,omitempty"`
	OSSessionID       *uint32             `json:"os_session_id,omitempty"`
	Connections       []DesktopConnection `json:"connections,omitempty"`
}

// ProviderInventory contains the protocol sessions returned by one VDI provider.
type ProviderInventory struct {
	SourceStatus
	Provider string            `json:"provider"`
	Sessions []ProtocolSession `json:"sessions,omitempty"`
}

// InventoryResponse is returned by the local system-probe VDI endpoint.
type InventoryResponse struct {
	CollectedAt time.Time                    `json:"collected_at"`
	Windows     WindowsInventory             `json:"windows"`
	Providers   map[string]ProviderInventory `json:"providers"`
}
