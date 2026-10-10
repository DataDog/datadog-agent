// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common defines shared types and structures for privileged logs functionality.
package common

// OpenFileRequest represents a request to open a file and transfer its file descriptor
type OpenFileRequest struct {
	Path     string `json:"path"`
	NoFollow bool   `json:"no_follow,omitempty"`
}

// UpgradeProtocol is the protocol the server switches the connection to in
// order to pass the opened file descriptor.
const UpgradeProtocol = "dd-privileged-logs"
