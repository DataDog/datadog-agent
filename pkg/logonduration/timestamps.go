// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logonduration provides boot and logon timestamp collection for macOS.
// This package is used by the system-probe module to collect login timestamps
// from OSLogStore, which requires root privileges.
package logonduration

import "time"

// LoginTimestamps contains the timestamps collected from system logs.
// These timestamps require root privileges to access OSLogStore.
type LoginTimestamps struct {
	// LoginWindowTime is when the login window appeared (user can enter credentials)
	LoginWindowTime *time.Time `json:"login_window_time,omitempty"`
	// LoginTime is when the user entered credentials (sessionDidLogin)
	LoginTime *time.Time `json:"login_time,omitempty"`
	// DesktopReadyTime is when the Dock checked in with launchservicesd (desktop ready)
	DesktopReadyTime *time.Time `json:"desktop_ready_time,omitempty"`
	// FileVaultEnabled indicates whether FileVault is enabled on the system
	FileVaultEnabled *bool `json:"filevault_enabled,omitempty"`
	// Error contains any error message if collection failed
	Error string `json:"error,omitempty"`
}
