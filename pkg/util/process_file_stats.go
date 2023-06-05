// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

// ProcessFileStats is used to retrieve stats from gopsutil/v3/process -- these stats are used for troubleshooting purposes
type ProcessFileStats struct {
	AgentOpenFiles uint64 `json:"agent_open_files"`
	OsFileLimit    uint64 `json:"os_file_limit"`
}
