// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package procfilestats provides a way to retrieve process open file stats
package procfilestats

import "errors"

// ErrNotImplemented is the "not implemented" error given by `gopsutil` when an
// OS doesn't support an API. Unfortunately it's in an internal package so
// we can't import it so we'll copy it here.
var ErrNotImplemented = errors.New("not implemented yet")

// ProcessFileStats is used to retrieve stats from gopsutil/v3/process -- these stats are used for troubleshooting purposes
type ProcessFileStats struct {
	AgentOpenFiles uint64 `json:"agent_open_files"`
	OsFileLimit    uint64 `json:"os_file_limit"`
}
