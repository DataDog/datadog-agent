// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"encoding/json"
	"errors"
)

// scanTaskPayload is the RC config received for a Data Security DB scan task.
// It mirrors the "DATA_SECURITY_DB_SCAN_TASKS" schema
type scanTaskPayload struct {
	TaskID string `json:"task_id"`
	// ScanningRules are dd-sds rules (including their license) forwarded to the
	// check verbatim as raw JSON.
	ScanningRules []json.RawMessage `json:"scanning_rules"`
	ScanData      []subTask         `json:"scan_data"`
}

// validate reports whether the payload is a well-formed Data Security scan task.
//
// TODO(data-security): the DEBUG product is generic, so it carries configs that are
// not scan tasks. Drop this check once we subscribe to the dedicated product.
func (p scanTaskPayload) validate() error {
	if p.TaskID == "" {
		return errors.New("missing task_id")
	}
	if len(p.ScanningRules) == 0 {
		return errors.New("missing scanning_rules")
	}
	if len(p.ScanData) == 0 {
		return errors.New("missing scan_data")
	}
	return nil
}
