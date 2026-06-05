// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common defines shared types and structures for privileged logs functionality.
package common

// OpenFileRequest represents a request to open a file and transfer its file descriptor
type OpenFileRequest struct {
	Path string `json:"path"`
	// NoFollow, when true, asks the module to open the file without following any
	// symbolic links in the path.  This is used for process_log-discovered paths,
	// which are canonical at discovery time; a symlink found later indicates an
	// attacker-controlled swap.  When false (the default), symbolic links are
	// resolved as usual.
	NoFollow bool `json:"no_follow,omitempty"`
}

// OpenFileResponse represents the response from the file descriptor transfer
type OpenFileResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
