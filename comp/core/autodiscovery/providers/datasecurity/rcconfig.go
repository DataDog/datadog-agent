// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import "encoding/json"

// scanTaskPayload is the RC config received for a Data Security DB scan task.
// It mirrors the "data-security-db-scan-tasks" schema: scanningRules are common
// to every sub task, scanData is the list of sub tasks to run against them.
type scanTaskPayload struct {
	TaskID string `json:"task_id"`
	// ScanningRules are dd-sds rules (including their license) forwarded to the
	// check verbatim as raw JSON.
	ScanningRules []json.RawMessage `json:"scanning_rules"`
	ScanData      []subTask         `json:"scan_data"`
}
